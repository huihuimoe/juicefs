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
	"os"
	"path/filepath"
	"testing"
)

func Test_readPasswordFromFile(t *testing.T) {
	// Create temporary directory for test files
	tempDir := t.TempDir()

	tests := []struct {
		name       string
		content    string
		filename   string
		createFile bool
		want       string
		wantErr    bool
	}{
		{
			name:       "valid password file",
			content:    "mypassword",
			filename:   "password.txt",
			createFile: true,
			want:       "mypassword",
			wantErr:    false,
		},
		{
			name:       "password with leading and trailing whitespace",
			content:    "\n  mypassword  \n\t",
			filename:   "password_with_spaces.txt",
			createFile: true,
			want:       "mypassword",
			wantErr:    false,
		},
		{
			name:       "empty file",
			content:    "",
			filename:   "empty.txt",
			createFile: true,
			want:       "",
			wantErr:    false,
		},
		{
			name:       "complex password with special characters",
			content:    "pa$$w0rd!@#",
			filename:   "complex.txt",
			createFile: true,
			want:       "pa$$w0rd!@#",
			wantErr:    false,
		},
		{
			name:       "file does not exist",
			content:    "",
			filename:   "nonexistent.txt",
			createFile: false,
			want:       "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filePath string
			if tt.createFile {
				filePath = filepath.Join(tempDir, tt.filename)
				err := os.WriteFile(filePath, []byte(tt.content), 0600)
				if err != nil {
					t.Fatalf("Failed to create test file %s: %v", filePath, err)
				}
			} else {
				filePath = filepath.Join(tempDir, tt.filename)
			}

			got, err := readPasswordFromFile(filePath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("readPasswordFromFile() expected error but got none")
					return
				}
			} else {
				if err != nil {
					t.Errorf("readPasswordFromFile() unexpected error = %v", err)
					return
				}
				if got != tt.want {
					t.Errorf("readPasswordFromFile() = %q, want %q", got, tt.want)
				}
			}
		})
	}
}
