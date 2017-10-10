package cacher

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/cybozu-go/aptutil/apt"
)

func makeFileInfo(path string, data []byte) (*apt.FileInfo, error) {
	rb := bytes.NewReader(data)
	wb := new(bytes.Buffer)
	fi, err := apt.CopyWithFileInfo(wb, rb, path)
	if err != nil {
		return nil, err
	}
	return fi, nil
}

func TestStorage(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cm := NewStorage(dir, 0)

	data := []byte{'a'}
	p := "path/to/a"
	fi, err := makeFileInfo(p, data)
	if err != nil {
		t.Fatal(err)
	}
	rb := bytes.NewReader(data)
	_, err = cm.Insert(rb, p, fi)
	if err != nil {
		t.Fatal(err)
	}
	if cm.Len() != 1 {
		t.Error(`cm.Len() != 1`)
	}
	if cm.used != 1 {
		t.Error(`cm.used != 1`)
	}

	// overwrite
	rb = bytes.NewReader(data)
	_, err = cm.Insert(rb, p, fi)
	if err != nil {
		t.Fatal(err)
	}
	if cm.Len() != 1 {
		t.Error(`cm.Len() != 1`)
	}
	if cm.used != 1 {
		t.Error(`cm.used != 1`)
	}

	data = []byte{'b', 'c'}
	p = "path/to/bc"
	fi, err = makeFileInfo(p, data)
	if err != nil {
		t.Fatal(err)
	}
	rb = bytes.NewReader(data)
	_, err = cm.Insert(rb, p, fi)
	if err != nil {
		t.Fatal(err)
	}
	if cm.Len() != 2 {
		t.Error(`cm.Len() != 2`)
	}
	if cm.used != 3 {
		t.Error(`cm.used != 3`)
	}

	data = []byte{'d', 'a', 't', 'a'}
	p = "data"
	fi, err = makeFileInfo(p, data)
	if err != nil {
		t.Fatal(err)
	}
	rb = bytes.NewReader(data)
	_, err = cm.Insert(rb, p, fi)
	if err != nil {
		t.Fatal(err)
	}
	f, err := cm.Lookup(fi)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data2, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(data, data2) != 0 {
		t.Error(`bytes.Compare(data, data2) != 0`)
	}

	differentData := []byte{'d', 'a', 't', '.'}
	fi, err = makeFileInfo("data", differentData)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cm.Lookup(fi)
	if err != ErrNotFound {
		t.Error(`err != ErrNotFound`)
	}

	err = cm.Delete("data")
	if err != nil {
		t.Fatal(err)
	}
	if cm.Len() != 2 {
		t.Error(`cm.Len() != 2`)
	}
	if cm.used != 3 {
		t.Error(`cm.used != 3`)
	}
}

func TestStorageLRU(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cm := NewStorage(dir, 3)

	dataA := []byte{'a'}
	pA := "path/to/a"
	fiA, err := makeFileInfo(pA, dataA)
	if err != nil {
		t.Fatal(err)
	}
	rb := bytes.NewReader(dataA)
	_, err = cm.Insert(rb, pA, fiA)
	if err != nil {
		t.Fatal(err)
	}

	dataBC := []byte{'b', 'c'}
	pBC := "path/to/bc"
	fiBC, err := makeFileInfo(pBC, dataBC)
	if err != nil {
		t.Fatal(err)
	}
	rb = bytes.NewReader(dataBC)
	_, err = cm.Insert(rb, pBC, fiBC)
	if err != nil {
		t.Fatal(err)
	}
	if cm.used != 3 {
		t.Error(`cm.used != 3`)
	}

	// a and bc will be purged
	dataDE := []byte{'d', 'e'}
	pDE := "path/to/de"
	fiDE, err := makeFileInfo(pDE, dataDE)
	if err != nil {
		t.Fatal(err)
	}
	rb = bytes.NewReader(dataDE)
	_, err = cm.Insert(rb, pDE, fiDE)
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

	rb = bytes.NewReader(dataA)
	_, err = cm.Insert(rb, pA, fiA)
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

func TestStoragePathTraversal(t *testing.T) {
	t.Parallel()

	dir, err := ioutil.TempDir("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cm := NewStorage(dir, 0)

	data := []byte{'a'}
	p := "/absolute/path"
	fi, err := makeFileInfo(p, data)
	if err != nil {
		t.Error(err)
	}
	rb := bytes.NewReader(data)
	_, err = cm.Insert(rb, p, fi)
	if err != ErrBadPath {
		t.Error(`/absolute/path must be a bad path`)
	}

	p = "./unclean/path"
	fi, err = makeFileInfo(p, data)
	if err != nil {
		t.Error(err)
	}
	rb = bytes.NewReader(data)
	_, err = cm.Insert(rb, p, fi)
	if err != ErrBadPath {
		t.Error(`./unclean/path must be a bad path`)
	}

	p = ""
	fi, err = makeFileInfo(p, data)
	if err != nil {
		t.Error(err)
	}
	rb = bytes.NewReader(data)
	_, err = cm.Insert(rb, p, fi)
	if err != ErrBadPath {
		t.Error(`empty path must be a bad path`)
	}

	p = "."
	fi, err = makeFileInfo(p, data)
	if err != nil {
		t.Error(err)
	}
	rb = bytes.NewReader(data)
	_, err = cm.Insert(rb, p, fi)
	if err != ErrBadPath {
		t.Error(`. must be a bad path`)
	}
}
