// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

//go:build race

package pfcpiface

// raceEnabled reports whether this binary was built with the race detector. See the
// !race build of this constant for why the allocation assertions consult it.
const raceEnabled = true
