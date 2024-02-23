// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

package healthv2

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cilium/cilium/pkg/healthv2/types"
	"github.com/cilium/cilium/pkg/hive"
	"github.com/cilium/cilium/pkg/hive/cell"
	"github.com/cilium/cilium/pkg/statedb"
)

func allStatus(db *statedb.DB, statusTable statedb.RWTable[types.Status]) []types.Status {
	ss := []types.Status{}
	tx := db.ReadTxn()
	iter, _ := statusTable.All(tx)
	for {
		s, _, ok := iter.Next()
		if !ok {
			break
		}
		ss = append(ss, s)
	}
	return ss
}

func byLevel(db *statedb.DB, statusTable statedb.RWTable[types.Status], l types.Level) []types.Status {
	ss := []types.Status{}
	tx := db.ReadTxn()
	q := LevelIndex.Query(l)
	iter, _ := statusTable.Get(tx, q)
	for {
		s, _, ok := iter.Next()
		if !ok {
			break
		}
		ss = append(ss, s)
	}
	return ss
}

func TestProvider(t *testing.T) {
	assert := assert.New(t)
	h := hive.New(
		statedb.Cell,
		cell.Provide(newHealthV2Provider),
		cell.ProvidePrivate(newTablesPrivate),
		cell.Invoke(func(statusTable statedb.RWTable[types.Status], db *statedb.DB, p types.Provider, sd hive.Shutdowner) error {
			h := p.ForModule(types.FullModuleID{"foo", "bar"})
			hm2 := p.ForModule(types.FullModuleID{"foo", "bar2"})
			hm2.NewScope("zzz").OK("yay2")

			h = h.NewScope("zzz")
			h.OK("yay")
			h.Degraded("noo", fmt.Errorf("err0"))

			h2 := h.NewScope("xxx")
			h2.OK("222")

			sd.Shutdown()
			all := allStatus(db, statusTable)
			assert.Len(all, 3)
			assert.Equal("foo.bar.zzz", all[0].ID.String())

			degraded := byLevel(db, statusTable, types.LevelDegraded)

			assert.Len(degraded, 1)
			assert.Equal(fmt.Errorf("err0"), degraded[0].Error)
			assert.Equal(degraded[0].Count, uint64(1))

			ok := byLevel(db, statusTable, types.LevelOK)
			assert.Len(ok, 2)

			assert.Len(byLevel(db, statusTable, types.LevelStopped), 0)

			h2.Stopped("done")
			all = allStatus(db, statusTable)

			for _, s := range all {
				if s.ID.String() == "foo.bar.zzz.xxx" {
					assert.NotZero(s.Stopped)
					continue
				}
				assert.Zero(s.Stopped)
			}
			return nil
		}),
	)
	assert.NoError(h.Run())
}
