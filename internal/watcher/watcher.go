package watcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/md-indexer/internal/service"
	"github.com/rs/zerolog/log"
)

type Watcher struct {
	fsWatcher *fsnotify.Watcher
	indexer   *service.IndexerService
	dir       string
}

func NewWatcher(dir string, indexer *service.IndexerService) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Add all subdirectories
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info != nil && info.IsDir() {
			return fsw.Add(path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Watcher{
		fsWatcher: fsw,
		indexer:   indexer,
		dir:       dir,
	}, nil
}

func (w *Watcher) Start() {
	for {
		select {
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			if !strings.HasSuffix(strings.ToLower(event.Name), ".md") {
				continue
			}

			ctx := context.Background()
			log.Info().Str("event", event.Op.String()).Str("file", event.Name).Msg("File system event")

			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				w.indexer.IndexFile(ctx, event.Name)
			} else if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				w.indexer.RemoveFile(ctx, event.Name)
			}
		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			log.Error().Err(err).Msg("Watcher error")
		}
	}
}

func (w *Watcher) Close() error {
	return w.fsWatcher.Close()
}
