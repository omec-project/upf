// SPDX-License-Identifier: Apache-2.0
// Copyright 2024 Canonical Ltd.

package pfcpiface

import (
	"errors"
	"math"
	"sync"
)

const (
	minValue = 1
	maxValue = math.MaxUint32
)

type FTEIDGenerator struct {
	lock    sync.Mutex
	offset  uint32
	usedMap map[uint32]bool
}

func NewFTEIDGenerator() *FTEIDGenerator {
	return &FTEIDGenerator{
		offset:  0,
		usedMap: make(map[uint32]bool),
	}
}

// Allocate and return an id in range [minValue, maxValue]
func (idGenerator *FTEIDGenerator) Allocate() (uint32, error) {
	idGenerator.lock.Lock()
	defer idGenerator.lock.Unlock()

	offsetBegin := idGenerator.offset
	for {
		if _, ok := idGenerator.usedMap[idGenerator.offset]; ok {
			idGenerator.updateOffset()

			if idGenerator.offset == offsetBegin {
				return 0, errors.New("no available value range to allocate id")
			}
		} else {
			break
		}
	}
	idGenerator.usedMap[idGenerator.offset] = true
	id := idGenerator.offset + minValue
	idGenerator.updateOffset()
	return id, nil
}

func (idGenerator *FTEIDGenerator) FreeID(id uint32) {
	if id < minValue {
		return
	}
	idGenerator.lock.Lock()
	defer idGenerator.lock.Unlock()
	delete(idGenerator.usedMap, id-minValue)
}

func (idGenerator *FTEIDGenerator) IsAllocated(id uint32) bool {
	if id < minValue {
		return false
	}
	idGenerator.lock.Lock()
	defer idGenerator.lock.Unlock()
	_, ok := idGenerator.usedMap[id-minValue]
	return ok
}

func (idGenerator *FTEIDGenerator) updateOffset() {
	idGenerator.offset++
	idGenerator.offset = idGenerator.offset % maxValue
}
