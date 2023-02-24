package badger3store

// Copyright 2019 Vivino. All rights reserved
//
// See LICENSE file for license details

import (
	"context"
	"fmt"
	"time"

	"github.com/Vivino/rankdb/blobstore"
	"github.com/Vivino/rankdb/log"
	"github.com/dgraph-io/badger/v3"
)

// New returns a new BadgerStore with the provided options.
func New(opts badger.Options) (*BadgerStore, error) {
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	b := &BadgerStore{
		db: db,
	}
	go b.startGC()
	return b, nil
}

// BadgerStore returns a store that operate on Badger database.
// We have a short queue to be able to do bulk writes.
type BadgerStore struct {
	db *badger.DB
}

func (b *BadgerStore) Close() error {
	return b.db.Close()
}

func (b *BadgerStore) key(set, key string) []byte {
	return []byte(set + "/" + key)
}

func (b *BadgerStore) Get(ctx context.Context, set, key string) ([]byte, error) {
	var res []byte
	err := b.db.View(func(tx *badger.Txn) error {
		item, err := tx.Get(b.key(set, key))
		if err != nil {
			return err
		}
		// blob only valid during transaction, so copy.
		res, err = item.ValueCopy(nil)
		return err
	})
	if err == badger.ErrKeyNotFound {
		err = blobstore.ErrBlobNotFound
	}
	return res, err
}

func (b *BadgerStore) Set(ctx context.Context, set, key string, val []byte) error {
	return b.db.Update(func(tx *badger.Txn) error {
		err := tx.Set(b.key(set, key), val)
		if err == badger.ErrKeyNotFound {
			err = nil
		}
		return err
	})
}

func (b *BadgerStore) Delete(ctx context.Context, set, key string) error {
	return b.db.Update(func(tx *badger.Txn) error {
		err := tx.Delete(b.key(set, key))
		if err == badger.ErrKeyNotFound {
			err = nil
		}
		return err
	})
}

func (b *BadgerStore) startGC() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
	again:
		err := b.db.RunValueLogGC(0.5)
		if err == nil {
			goto again
		}
		if err == badger.ErrRejected {
			return
		}
	}
}

// BadgerLogger converts a logger on a context to a Badger logger.
// Debug messages are ignored, Info are forwarded and Warning/Errors are written as errors.
func BadgerLogger(ctx context.Context) badger.Logger {
	return logWrapper{log.Logger(ctx)}
}

type logWrapper struct {
	a log.Adapter
}

func (b logWrapper) Errorf(s string, vals ...interface{}) {
	b.a.Error(fmt.Sprintf(s, vals...))
}

func (b logWrapper) Warningf(s string, vals ...interface{}) {
	b.a.Error(fmt.Sprintf(s, vals...))
}

func (b logWrapper) Infof(s string, vals ...interface{}) {
	b.a.Info(fmt.Sprintf(s, vals...))
}

func (b logWrapper) Debugf(string, ...interface{}) {}
