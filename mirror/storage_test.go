package mirror

import (
	"bytes"
	"io/ioutil"
	"os"
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

func testStorageBadConstruction(t *testing.T) {
	t.Parallel()

	f, err := ioutil.TempFile("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer func(f string) {
		if _, err := os.Stat(f); err != nil {
			return
		}
		os.Remove(f)
	}(f.Name())

	_, err = NewStorage(f.Name(), "pre")
	if err == nil {
		t.Error("NewStorage must fail with regular file")
	}

	os.Remove(f.Name())
	_, err = NewStorage(f.Name(), "pre")
	if err == nil {
		t.Error("NewStorage must fail with non-existent directory")
	}
}

func testStorageLookup(t *testing.T) {
	t.Parallel()

	d, err := ioutil.TempDir("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)

	s, err := NewStorage(d, "pre")
	if err != nil {
		t.Fatal(err)
	}

	err = s.Load()
	if err != nil {
		t.Error(err)
	}

	files := map[string][]byte{
		"a/b/c":   {'a', 'b', 'c'},
		"def":     {'d', 'e', 'f'},
		"a/pp/le": {'a', 'p', 'p', 'l', 'e'},
	}

	for fn, data := range files {
		fi, err := makeFileInfo(fn, data)
		if err != nil {
			t.Fatal(err)
		}

		rb := bytes.NewReader(data)
		if err := s.Store(fn, fi, rb); err != nil {
			t.Fatal(err)
		}
	}

	fi, err := makeFileInfo("a/b/c", []byte{'a', 'b', 'd'})
	if err != nil {
		t.Fatal(err)
	}
	fi2, fullpath := s.Lookup(fi, false)
	if fi2 != nil {
		t.Error(`fi2 != nil`)
	}
	if len(fullpath) != 0 {
		t.Error(`len(fullpath) != 0`)
	}

	fi, err = makeFileInfo("a/b/c", files["a/b/c"])
	if err != nil {
		t.Fatal(err)
	}
	fi3, _ := s.Lookup(fi, false)
	if fi3 == nil {
		t.Error(`fi3 == nil`)
	}

	s.Save()

	s2, err := NewStorage(d, "ubuntu")
	if err != nil {
		t.Fatal(err)
	}

	err = s2.Load()
	if err != nil {
		t.Error(err)
	}

	fi, err = makeFileInfo("a/b/c", files["a/b/c"])
	if err != nil {
		t.Fatal(err)
	}
	fi4, _ := s2.Lookup(fi, false)
	if fi4 == nil {
		t.Error(`fi4 == nil`)
	}

	fi, err = makeFileInfo("def", files["def"])
	if err != nil {
		t.Fatal(err)
	}
	fi5, _ := s2.Lookup(fi, false)
	if fi5 == nil {
		t.Error(`fi5 == nil`)
	}

	fi, err = makeFileInfo("a/pp/le", files["a/pp/le"])
	if err != nil {
		t.Fatal(err)
	}
	fi6, _ := s2.Lookup(fi, false)
	if fi6 == nil {
		t.Error(`fi6 == nil`)
	}

	fi, err = makeFileInfo("a/pp/le", files["def"])
	if err != nil {
		t.Fatal(err)
	}
	fi7, _ := s2.Lookup(fi, false)
	if fi7 != nil {
		t.Error(`fi7 != nil`)
	}
}

func testStorageStore(t *testing.T) {
	t.Parallel()

	d, err := ioutil.TempDir("", "gotest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)

	s, err := NewStorage(d, "pre")
	if err != nil {
		t.Fatal(err)
	}

	err = s.Load()
	if err != nil {
		t.Error(err)
	}

	fn := "a/b/c"
	data := []byte{'a', 'b', 'c'}
	fi, err := makeFileInfo(fn, data)
	if err != nil {
		t.Fatal(err)
	}

	rb := bytes.NewReader(data)
	err = s.Store(fn, fi, rb)
	if err != nil {
		t.Error(err)
	}
	found, _ := s.Lookup(fi, false)
	if found == nil {
		t.Error(`found == nil`)
	}

	// duplicates should not be granted
	rb = bytes.NewReader(data)
	err = s.Store(fn, fi, rb)
	if err == nil {
		t.Error(`err == nil`)
	}

	data2 := []byte{'d', 'e', 'f'}
	fi2, err := makeFileInfo(fn, data2)
	if err != nil {
		t.Fatal(err)
	}

	rb = bytes.NewReader(data2)
	err = s.StoreWithHash(fn, fi2, rb)
	if err != nil {
		t.Error(err)
	}
	notfound, _ := s.Lookup(fi2, false)
	if notfound != nil {
		t.Error(`notfound != nil`)
	}
	found, _ = s.Lookup(fi2, true)
	if found == nil {
		t.Error(`found == nil`, d, fi2.SHA256Path())
	}
}

func TestStorage(t *testing.T) {
	t.Run("BadConstruction", testStorageBadConstruction)
	t.Run("Lookup", testStorageLookup)
	t.Run("Store", testStorageStore)
}
