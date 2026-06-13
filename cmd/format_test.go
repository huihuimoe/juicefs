/*
 * JuiceFS, Copyright 2020 Juicedata, Inc.
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

package cmd

import (
	"os"
	"testing"

	"github.com/juicedata/juicefs/pkg/meta"
)

const memKVSettingPath = "/tmp/juicefs.memkv.setting.json"

func TestFixObjectSize(t *testing.T) {
	t.Run("Should make sure the size is in range", func(t *testing.T) {
		cases := []struct {
			input, expected uint64
		}{
			{30 << 10, 64 << 10},
			{0, 64 << 10},
			{2 << 40, 16 << 20},
			{16 << 21, 16 << 20},
		}
		for _, c := range cases {
			if size := fixObjectSize(c.input); size != c.expected {
				t.Fatalf("Expected %d, got %d", c.expected, size)
			}
		}
	})
	t.Run("Should use powers of two", func(t *testing.T) {
		cases := []struct {
			input, expected uint64
		}{
			{150 << 10, 128 << 10},
			{99 << 10, 64 << 10},
			{1077 << 10, 1024 << 10},
		}
		for _, c := range cases {
			if size := fixObjectSize(c.input); size != c.expected {
				t.Fatalf("Expected %d, got %d", c.expected, size)
			}
		}
	})
}

func TestFormat(t *testing.T) {
	_ = os.Remove(memKVSettingPath)
	t.Cleanup(func() { _ = os.Remove(memKVSettingPath) })

	metaURL := "memkv://"
	if err := Main([]string{"", "format", "--bucket", t.TempDir(), metaURL, testVolume}); err != nil {
		t.Fatalf("format error: %s", err)
	}
	m := meta.NewClient(metaURL, nil)
	f, err := m.Load(false)
	if err != nil {
		t.Fatalf("load format: %s", err)
	}
	if f.Name != testVolume {
		t.Fatalf("volume name %s != expected %s", f.Name, testVolume)
	}
	if err = m.Shutdown(); err != nil {
		t.Fatalf("shutdown metadata: %s", err)
	}

	if err = Main([]string{"", "format", metaURL, testVolume, "--capacity", "1", "--inodes", "1000"}); err != nil {
		t.Fatalf("format error: %s", err)
	}
	m = meta.NewClient(metaURL, nil)
	defer m.Shutdown()
	if f, err = m.Load(false); err != nil {
		t.Fatalf("load format: %s", err)
	}
	if f.Capacity != 1<<30 || f.Inodes != 1000 {
		t.Fatalf("unexpected volume: %+v", f)
	}
}
