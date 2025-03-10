//go:build linux
// +build linux

/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package fifo

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFifoCloseAfterRm(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "fifos")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	leakCheckWg = &sync.WaitGroup{}
	defer func() {
		leakCheckWg = nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	f, err := OpenFifo(ctx, filepath.Join(tmpdir, "f0"), syscall.O_RDONLY|syscall.O_CREAT|syscall.O_NONBLOCK, 0600)
	assert.NoError(t, err)

	err = os.RemoveAll(filepath.Join(tmpdir, "f0"))
	assert.NoError(t, err)

	cerr := make(chan error)

	go func() {
		b := make([]byte, 32)
		_, err := f.Read(b)
		cerr <- err
	}()

	select {
	case err := <-cerr:
		t.Fatalf("read should have blocked, but got %v", err)
	case <-time.After(500 * time.Millisecond):
	}

	err = f.Close()
	assert.NoError(t, err)

	select {
	case err := <-cerr:
		assert.EqualError(t, err, "reading from a closed fifo")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("read should have been unblocked")
	}

	assert.NoError(t, checkWgDone(leakCheckWg))

	cancel()
	ctx, cancel = context.WithCancel(context.Background())

	cerr = make(chan error)
	go func() {
		_, err = OpenFifo(ctx, filepath.Join(tmpdir, "f1"), syscall.O_WRONLY|syscall.O_CREAT, 0600)
		cerr <- err
	}()

	select {
	case err := <-cerr:
		t.Fatalf("open should have blocked, but got %v", err)
	case <-time.After(500 * time.Millisecond):
	}

	err = os.RemoveAll(filepath.Join(tmpdir, "f1"))
	cancel()

	select {
	case err := <-cerr:
		assert.EqualError(t, err, "context canceled")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("open should have been unblocked")
	}

	assert.NoError(t, checkWgDone(leakCheckWg))
}
