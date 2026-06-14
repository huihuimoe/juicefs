//go:build !nobadger
// +build !nobadger

/*
 * JuiceFS, Copyright 2022 Juicedata, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package meta

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/juicedata/juicefs/pkg/utils"
)

type badgerTxn struct {
	t      *badger.Txn
	c      *badgerClient
	gcSize int
}

func (tx *badgerTxn) id() uint64 {
	// add logical id to avoid conflict between concurrent transactions
	return tx.t.ReadTs()*1e2 + tx.c.getId()%1e2
}

func (tx *badgerTxn) get(key []byte) []byte {
	item, err := tx.t.Get(key)
	if err == badger.ErrKeyNotFound {
		return nil
	}
	if err != nil {
		panic(err)
	}
	value, err := item.ValueCopy(nil)
	if err != nil {
		panic(err)
	}
	return value
}

func (tx *badgerTxn) gets(keys ...[]byte) [][]byte {
	values := make([][]byte, len(keys))
	for i, key := range keys {
		values[i] = tx.get(key)
	}
	return values
}

func (tx *badgerTxn) scan(begin, end []byte, keysOnly bool, handler func(k, v []byte) bool) {
	var prefix bool
	options := badger.IteratorOptions{}
	if keysOnly {
		options.PrefetchValues = false
		options.PrefetchSize = 0
	}
	if bytes.Equal(nextKey(begin), end) {
		prefix = true
		options.Prefix = begin
	}
	it := tx.t.NewIterator(options)
	if prefix {
		it.Rewind()
	} else {
		it.Seek(begin)
	}
	defer it.Close()
	for ; it.Valid(); it.Next() {
		item := it.Item()
		if !prefix && bytes.Compare(item.Key(), end) >= 0 {
			break
		}
		var value []byte
		if !keysOnly {
			var err error
			value, err = item.ValueCopy(nil)
			if err != nil {
				panic(err)
			}
		}
		if !handler(item.KeyCopy(nil), value) {
			break
		}
	}
}

func (tx *badgerTxn) exist(prefix []byte) bool {
	it := tx.t.NewIterator(badger.IteratorOptions{
		Prefix:       prefix,
		PrefetchSize: 1,
	})
	defer it.Close()
	it.Rewind()
	return it.Valid()
}

func (tx *badgerTxn) set(key, value []byte) {
	if err := tx.t.Set(key, value); err != nil {
		panic(err)
	}
	tx.gcSize += len(key) + len(value)
}

func (tx *badgerTxn) append(key []byte, value []byte) {
	list := append(tx.get(key), value...)
	tx.set(key, list)
}

func (tx *badgerTxn) incrBy(key []byte, value int64) int64 {
	buf := tx.get(key)
	newCounter := parseCounter(buf)
	if value != 0 {
		newCounter += value
		tx.set(key, packCounter(newCounter))
	}
	return newCounter
}

func (tx *badgerTxn) delete(key []byte) {
	if err := tx.t.Delete(key); err != nil {
		panic(err)
	}
	tx.gcSize += len(key)
}

type badgerClient struct {
	client   *badger.DB
	ticker   *time.Ticker
	done     chan struct{}
	nextid   uint64
	gcSize   uint64
	gcPolicy badgerGCPolicy
}

func (c *badgerClient) name() string {
	return "badger"
}

func (c *badgerClient) getId() uint64 {
	return atomic.AddUint64(&c.nextid, 1)
}

func (c *badgerClient) rewind(id uint64, factor int) uint64 {
	shift := uint64(1e5)
	if s := os.Getenv("JFS_TKV_REWIND"); s != "" {
		if parsed, err := strconv.ParseUint(s, 10, 64); err == nil && parsed > 0 {
			shift = parsed
		}
	}
	if factor > 1 {
		shift *= uint64(factor)
	}
	if id > shift {
		return id - shift
	}
	return 1
}

func (c *badgerClient) shouldRetry(err error) bool {
	return err == badger.ErrConflict
}

func (c *badgerClient) config(key string) interface{} {
	return nil
}

func (c *badgerClient) trackGCSize(size int) {
	if c.gcPolicy.triggerSize > 0 {
		atomic.AddUint64(&c.gcSize, uint64(size))
	}
}

func (c *badgerClient) shouldRunGC() bool {
	if c.gcPolicy.triggerSize == 0 {
		return true
	}
	for {
		size := atomic.LoadUint64(&c.gcSize)
		if size < c.gcPolicy.triggerSize {
			return false
		}
		if atomic.CompareAndSwapUint64(&c.gcSize, size, 0) {
			return true
		}
	}
}

func (c *badgerClient) simpleTxn(ctx context.Context, f func(*kvTxn) error, retry int) (err error) {
	return c.txn(ctx, f, retry)
}

func (c *badgerClient) txn(ctx context.Context, f func(*kvTxn) error, retry int) (err error) {
	tx := &badgerTxn{t: c.client.NewTransaction(true), c: c}
	defer func() { tx.t.Discard() }()
	defer func() {
		if r := recover(); r != nil {
			fe, ok := r.(error)
			if ok {
				err = fe
			} else {
				panic(r)
			}
		}
	}()
	err = f(&kvTxn{tx, retry})
	if err != nil {
		return err
	}
	// tx.t may differ from the original
	err = tx.t.Commit()
	if err == nil {
		tx.c.trackGCSize(tx.gcSize)
	}
	return err
}

func (c *badgerClient) scan(prefix []byte, handler func(key []byte, value []byte) bool) error {
	tx := c.client.NewTransaction(false)
	defer tx.Discard()
	it := tx.NewIterator(badger.IteratorOptions{
		Prefix:         prefix,
		PrefetchValues: true,
		PrefetchSize:   10240,
	})
	defer it.Close()
	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		value, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		if !handler(item.KeyCopy(nil), value) {
			break
		}
	}
	return nil
}

func (c *badgerClient) reset(prefix []byte) error {
	if prefix == nil {
		return c.client.DropAll()
	}
	return c.client.DropPrefix(prefix)
}

func (c *badgerClient) close() error {
	close(c.done)
	c.ticker.Stop()
	return c.client.Close()
}

func (c *badgerClient) gc() {}

const (
	badgerProfileDefault = "default"
	badgerProfileLean    = "lean"

	badgerLeanBlockCacheSize = 32 << 20
	badgerLeanIndexCacheSize = 64 << 20
	badgerLeanMemTableSize   = 16 << 20
	badgerLeanNumMemtables   = 2
	badgerLeanNumCompactors  = 2
	badgerLeanGCTriggerSize  = 64 << 20
)

type badgerGCPolicy struct {
	interval    time.Duration
	triggerSize uint64
}

func badgerOptions(addr string) (badger.Options, badgerGCPolicy, error) {
	path, profile, err := parseBadgerAddr(addr)
	if err != nil {
		return badger.Options{}, badgerGCPolicy{}, err
	}

	opt := badger.DefaultOptions(path)
	gc := badgerGCPolicy{interval: time.Hour}
	switch profile {
	case badgerProfileDefault:
	case badgerProfileLean:
		opt.BlockCacheSize = badgerLeanBlockCacheSize
		opt.IndexCacheSize = badgerLeanIndexCacheSize
		opt.MemTableSize = badgerLeanMemTableSize
		opt.NumMemtables = badgerLeanNumMemtables
		opt.NumCompactors = badgerLeanNumCompactors
		gc.interval = time.Minute
		gc.triggerSize = badgerLeanGCTriggerSize
	default:
		return badger.Options{}, badgerGCPolicy{}, fmt.Errorf("unsupported badger profile %q", profile)
	}
	return opt, gc, nil
}

func parseBadgerAddr(addr string) (string, string, error) {
	path, rawQuery, ok := strings.Cut(addr, "?")
	if !ok {
		return addr, badgerProfileDefault, nil
	}
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", "", fmt.Errorf("parse badger options: %w", err)
	}
	profile := q.Get("profile")
	q.Del("profile")
	if len(q) > 0 {
		for key := range q {
			return "", "", fmt.Errorf("unsupported badger option %q", key)
		}
	}
	if profile == "" {
		profile = badgerProfileDefault
	}
	return path, profile, nil
}

func newBadgerClient(addr string) (tkvClient, error) {
	opt, gc, err := badgerOptions(addr)
	if err != nil {
		return nil, err
	}
	opt.Logger = utils.GetLogger("badger")
	opt.MetricsEnabled = false
	client, err := badger.Open(opt)
	if err != nil {
		return nil, err
	}
	ticker := time.NewTicker(gc.interval)
	done := make(chan struct{})
	c := &badgerClient{client: client, ticker: ticker, done: done, gcPolicy: gc}
	go func() {
		for {
			select {
			case <-ticker.C:
				if !c.shouldRunGC() {
					continue
				}
				for c.client.RunValueLogGC(0.7) == nil {
				}
			case <-done:
				return
			}
		}
	}()
	return c, nil
}

func init() {
	Register("badger", newKVMeta)
	drivers["badger"] = newBadgerClient
}
