package file

import (
	"io/ioutil"
	"os"
	"time"
)

// TailerT is an interface representing a Tailer (useful for mocks).
type TailerT interface {
	Next() ([]byte, error)
}

// Tailer is an abstraction for reading newly appended content from a file,
// implementing TailerT (i.e. returning newly appended bytes on calls to
// Next()). After idleDuration of file inactivity (no new content), calls to
// Next() also invoke a rotation check.
type Tailer struct {
	path         string
	file         *os.File
	fileInfo     os.FileInfo
	lastContent  time.Time
	idleDuration time.Duration
}

// NewTailer creates a new Tailer object configured to read data from the file
// at the supplied path (and performing rotation checks after idleDuration of
// inactivity).
func NewTailer(path string, idleDuration time.Duration) (*Tailer, error) {
	t := &Tailer{
		path:         path,
		idleDuration: idleDuration,
	}
	if err := t.openOrRotate(); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *Tailer) openOrRotate() error {
	file, err := os.Open(t.path)
	if err != nil {
		return err
	}

	info, err := file.Stat()
	if err != nil {
		return err
	}

	if t.file == nil {
		// First time, just open.
		t.file = file
		t.fileInfo = info
	} else if os.SameFile(info, t.fileInfo) {
		// Later check, same file.
		file.Close()
	} else {
		// Later check, rotation detected.
		t.file.Close()
		t.file = file
		t.fileInfo = info
	}
	return nil
}

// Next will return content newly read from the log file. If no new content is
// available, and this condition has persisted for at least the idleDuration, a
// rotation check will be performed.
func (t *Tailer) Next() ([]byte, error) {
	bytes, err := ioutil.ReadAll(t.file)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	if len(bytes) > 0 {
		t.lastContent = now
	} else if now.Sub(t.lastContent) > t.idleDuration {
		t.openOrRotate()
	}

	return bytes, nil
}
