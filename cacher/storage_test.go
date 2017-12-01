package cacher

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cybozu-go/aptutil/apt"
)

func insert(cm *Storage, data []byte, path string) (*apt.FileInfo, error) {
	f, err := cm.TempFile()
	if err != nil {
		return nil, err
	}
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()

	fi, err := apt.CopyWithFileInfo(f, bytes.NewReader(data), path)
	if err != nil {
		return nil, err
	}

	err = f.Sync()
	if err != nil {
		return nil, err
	}

	err = cm.Insert(f.Name(), fi)
	return fi, err
}

func makeFileInfo(path string, data []byte) (*apt.FileInfo, error) {
	rb := bytes.NewReader(data)
	wb := new(bytes.Buffer)
	fi, err := apt.CopyWithFileInfo(wb, rb, path)
	if err != nil {
		return nil, err
	}
	return fi, nil
}

func testStorageInsertWorksCorrectly(t *testing.T) {
	t.Parallel()
	dir, err := ioutil.TempDir("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	cm := NewStorage(dir, 0)

	fi, err = insert(cm, "a", "path/to/a")
	if err != nil {
		t.Fatal(err)
	}

	if cm.Len() != 1 {
		t.Error(`cm.Len() != 1`)
	}

	_, err = cm.Lookup(fi)
	if err != nil {
		t.Error(`cannot lookup inserted file`)
	}
}

func testStorageInsertOverwrite(t *testing.T) {
	t.Parallel()
	dir, err := ioutil.TempDir("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	cm := NewStorage(dir, 0)

	fi, err = insert(cm, "a", "path/to/a")
	if err != nil {
		t.Fatal(err)
	}

	fi, err = insert(cm, "a", "path/to/a")
	if err != nil {
		t.Fatal(err)
	}

	if cm.Len() != 1 {
		t.Error(`cm.Len() != 1`)
	}

	_, err = cm.Lookup(fi)
	if err != nil {
		t.Error(`cannot lookup inserted file`)
	}
}

func testStorageInsertReturnsErrorAgainstBadPath(t *testing.T) {
	t.Parallel()
	dir, err := ioutil.TempDir("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	cm := NewStorage(dir, 0)

	cases := []struct{ Title, Path string }{
		{
			Title: "Absolute path",
			Path:  "/absolute/path",
		},
		{
			Title: "Uncleaned path",
			Path:  "./uncleaned/path",
		},
		{
			Title: "Empty path",
			Path:  "",
		},
		{
			Title: ".",
			Path:  ".",
		},
	}

	for tc := range cases {
		t.Run(tc.Title, func() {
			_, err = insert(cm, []byte("a"), tc.Path)
			if err != ErrBadPath {
				t.Fatal(err)
			}
		})
	}
}

func testStorageInsertPurgesFilesAllowingLRU(t *testing.T) {
	t.Parallel()
	dir, err := ioutil.TempDir("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	cm := NewStorage(dir, 0)

	fiA, err := insert(cm, []byte("a"), "a")
	if err != nil {
		t.Fatal(err)
	}

	fiBC, err := insert(cm, []byte("bc"), "bc")
	if err != nil {
		t.Fatal(err)
	}

}

func TestStorageInsert(t *testing.T) {
	t.Run("Storage.Insert should insert data", testStorageInsertWorksCorrectly)
	t.Run("Storage.Insert should overwrite", testStorageInsertOverwrite)
	t.Run("Storage.Insert should return error if passed FileInfo path is bad path", testStorageInsertReturnsErrorAgainstBadPath)
	t.Run("Storage.Insert should purge files allowing LRU", testStorageInsertPurgesFilesAllowingLRU)
}

func TestStorageLRU(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cm := NewStorage(dir, 3)

	fA, err := storage.TempFile()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		fA.Close()
		os.Remove(fA.Name())
	}()

	pA := "path/to/a"
	srcA := strings.NewReader("a")
	fiA, err := apt.CopyWithFileInfo(fA, srcA, pA)
	if err != nil {
		t.Fatal(err)
	}
	err = cm.Insert(fA.Name(), fiA)
	if err != nil {
		t.Fatal(err)
	}

	fBC, err := storage.TempFile()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		fBC.Close()
		os.Remove(fBC.Name())
	}()

	pBC := "path/to/bc"
	srcBC := strings.NewReader("bc")
	fiBC, err := apt.CopyWithFileInfo(fBC, srcBC, pBC)
	if err != nil {
		t.Fatal(err)
	}
	err = cm.Insert(fBC.Name(), fiBC)
	if err != nil {
		t.Fatal(err)
	}
	if cm.used != 3 {
		t.Error(`cm.used != 3`)
	}

	// a and bc will be purged
	fDE, err := storage.TempFile()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		fDE.Close()
		os.Remove(fDE.Name())
	}()

	pDE := "path/to/de"
	srcDE := strings.NewReader("de")
	fiDE, err := apt.CopyWithFileInfo(fDE, srcDE, pDE)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cm.Insert(fDE.Name(), fiDE)
	if err != nil {
		t.Fatal(err)
	}
	if cm.Len() != 1 {
		t.Error(`cm.Len() != 1`)
	}
	if cm.used != 2 {
		t.Error(`cm.used != 2`)
	}

	_, err = cm.Lookup(fiA)
	if err != ErrNotFound {
		t.Error(`err != ErrNotFound`)
	}
	_, err = cm.Lookup(fiBC)
	if err != ErrNotFound {
		t.Error(`err != ErrNotFound`)
	}

	_, err = cm.Insert(fA.Name(), fiA)
	if err != nil {
		t.Fatal(err)
	}

	// touch de
	_, err = cm.Lookup(fiDE)
	if err != nil {
		t.Error(err)
	}

	// a will be purged
	dataF := []byte{'f'}
	pF := "path/to/f"
	fiF, err := makeFileInfo(pF, dataF)
	if err != nil {
		t.Fatal(err)
	}
	rb = bytes.NewReader(dataF)
	_, err = cm.Insert(rb, pF, fiF)
	if err != nil {
		t.Fatal(err)
	}

	_, err = cm.Lookup(fiA)
	if err != ErrNotFound {
		t.Error(`err != ErrNotFound`)
	}
	_, err = cm.Lookup(fiDE)
	if err != nil {
		t.Error(err)
	}
	_, err = cm.Lookup(fiF)
	if err != nil {
		t.Error(err)
	}
}

func TestStorageLoad(t *testing.T) {
	t.Parallel()

	files := map[string][]byte{
		"a":    {'a'},
		"bc":   {'b', 'c'},
		"def":  {'d', 'e', 'f'},
		"ghij": {'g', 'h', 'i', 'j'},
	}

	dir, err := ioutil.TempDir("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	for k, v := range files {
		err := ioutil.WriteFile(filepath.Join(dir, k+fileSuffix), v, 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	// dummy should be ignored as it does not have a proper suffix.
	err = ioutil.WriteFile(filepath.Join(dir, "dummy"), []byte{'d'}, 0644)
	if err != nil {
		t.Fatal(err)
	}

	cm := NewStorage(dir, 0)
	cm.Load()

	l := cm.ListAll()
	if len(l) != len(files) {
		t.Error(`len(l) != len(files)`)
	}

	fi, err := makeFileInfo("a", files["a"])
	if err != nil {
		t.Error(err)
	}
	f, err := cm.Lookup(fi)
	if err != nil {
		t.Error(err)
	}
	f.Close()
	fi, err = makeFileInfo("bc", files["bc"])
	if err != nil {
		t.Error(err)
	}
	f, err = cm.Lookup(fi)
	if err != nil {
		t.Error(err)
	}
	f.Close()
	fi, err = makeFileInfo("def", files["def"])
	if err != nil {
		t.Error(err)
	}
	f, err = cm.Lookup(fi)
	if err != nil {
		t.Error(err)
	}
	f.Close()

	fi, err = makeFileInfo("ghij", files["ghij"])
	if err != nil {
		t.Error(err)
	}
	f, err = cm.Lookup(fi)
	if err != nil {
		t.Fatal(err)
	}

	data, err := ioutil.ReadAll(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(files["ghij"], data) != 0 {
		t.Error(`bytes.Compare(files["ghij"], data) != 0`)
	}
}
