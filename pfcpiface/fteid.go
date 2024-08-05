// SPDX-License-Identifier: Apache-2.0
// Copyright 2024 Canonical Ltd.

package pfcpiface

import (
	"errors"
	"sync"
)

type FTEIDGenerator struct {
	lock       sync.Mutex
	minValue   uint32
	maxValue   uint32
	valueRange uint32
	offset     uint32
	usedMap    map[uint32]bool
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
	id := idGenerator.offset + idGenerator.minValue
	idGenerator.updateOffset()
	return id, nil
}

func (idGenerator *FTEIDGenerator) FreeID(id uint32) {
	if id < idGenerator.minValue || id > idGenerator.maxValue {
		return
	}
	idGenerator.lock.Lock()
	defer idGenerator.lock.Unlock()
	delete(idGenerator.usedMap, id-idGenerator.minValue)
}

func (idGenerator *FTEIDGenerator) updateOffset() {
	idGenerator.offset++
	idGenerator.offset = idGenerator.offset % idGenerator.valueRange
}
