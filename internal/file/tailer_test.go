package file_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/swfrench/nginx-log-exporter/internal/file"
)

type RotatingTempFile struct {
	Name  string
	File  *os.File
	count int
}

func NewRotatingTempFile(base string) (*RotatingTempFile, error) {
	newFile, err := ioutil.TempFile("", base)
	if err != nil {
		return nil, err
	}

	return &RotatingTempFile{
		Name:  newFile.Name(),
		File:  newFile,
		count: 0,
	}, nil
}

func (s *RotatingTempFile) AllTempFileNames() []string {
	fileNames := []string{s.Name}
	for i := 0; i < s.count; i++ {
		fileNames = append(fileNames, fmt.Sprintf("%s.%v", s.Name, i))
	}
	return fileNames
}

func (s *RotatingTempFile) Rotate() error {
	s.count += 1

	names := s.AllTempFileNames()
	for i := len(names) - 1; i > 0; i-- {
		if err := os.Rename(names[i-1], names[i]); err != nil {
			return err
		}
	}

	if err := s.File.Close(); err != nil {
		return err
	}

	newFile, err := os.Create(s.Name)
	if err != nil {
		return err
	}
	s.File = newFile

	return nil
}

func TestErrorNoFile(t *testing.T) {
	const testFile = "/this/will/never/exist"
	_, err := file.NewTailer(testFile, time.Second)
	if err == nil {
		t.Fatalf("Expected NewTailer to return an error")
	}
}

func syncWrite(f *os.File, content []byte) error {
	if _, err := f.Write(content); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return nil
}

func TestRead(t *testing.T) {
	testContent := [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}

	logFile, err := ioutil.TempFile("", "test_log_file")
	if err != nil {
		t.Fatalf("Could not open test log file: %v", logFile)
	}
	defer os.Remove(logFile.Name())

	tail, err := file.NewTailer(logFile.Name(), time.Second)
	if err != nil {
		t.Fatalf("Could not create tailer: %v", err)
	}

	for _, content := range testContent {
		if err := syncWrite(logFile, content); err != nil {
			t.Fatalf("Could not durably write to log file: %v", err)
		}

		b, err := tail.Next()
		if err != nil {
			t.Fatalf("Error fetching next byte slice: %v", err)
		}
		if want, got := content, b; !bytes.Equal(want, got) {
			t.Fatalf("Expected to read %v, got %v", want, got)
		}
	}

	b, err := tail.Next()
	if err != nil {
		t.Fatalf("Error fetching next byte slice: %v", err)
	}
	if len(b) > 0 {
		t.Fatalf("Expected zero-length content, got: %v", b)
	}

	if err := logFile.Close(); err != nil {
		t.Fatalf("Could not close log file")
	}
}

func TestReadRotate(t *testing.T) {
	rotate, err := NewRotatingTempFile("test_log_file")
	if err != nil {
		t.Fatalf("Could not initialize test log rotator: %v", err)
	}

	testIdleTime := 10 * time.Millisecond
	testContent := [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}

	tail, err := file.NewTailer(rotate.Name, testIdleTime)
	if err != nil {
		t.Fatalf("Could not create tailer: %v", err)
	}

	for iter, content := range testContent {
		if err := syncWrite(rotate.File, content); err != nil {
			t.Fatalf("Could not durably write to log file: %v", err)
		}

		if iter > 0 {
			// We know we've rotated: Expect one no-op Next() call.
			b, err := tail.Next()
			if err != nil {
				t.Fatalf("Error fetching next byte slice: %v", err)
			}
			if len(b) > 0 {
				t.Fatalf("Expected zero-length content following rotation, got: %v", b)
			}
		}

		// Second next call should pick up the newly written value.
		b, err := tail.Next()
		if err != nil {
			t.Fatalf("Error fetching next byte slice: %v", err)
		}
		if want, got := content, b; !bytes.Equal(want, got) {
			t.Fatalf("Expected to read %v, got %v", want, got)
		}

		if err = rotate.Rotate(); err != nil {
			t.Fatalf("Error rotating log file: %v", err)
		}
		time.Sleep(2 * testIdleTime)
	}

	b, err := tail.Next()
	if err != nil {
		t.Fatalf("Error fetching next byte slice: %v", err)
	}
	if len(b) > 0 {
		t.Fatalf("Expected zero-length content, got: %v", b)
	}

	for _, name := range rotate.AllTempFileNames() {
		err := os.Remove(name)
		if err != nil {
			t.Fatalf("Could not remove test log file: %v", err)
		}
	}
}
