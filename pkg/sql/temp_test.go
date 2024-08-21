// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package sql_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/sql/catalog/lease"
	"github.com/cockroachdb/cockroach/pkg/testutils/serverutils"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/stretchr/testify/require"
)

func TestTemp(t *testing.T) {
	defer leaktest.AfterTest(t)()
	defer log.Scope(t).Close(t)

	// The descriptor changes made must have an immediate effect
	// so disable leases on tables.
	defer lease.TestingDisableTableLeases()()

	ctx := context.Background()
	params, _ := createTestServerParams()
	s, sqlDB, _ := serverutils.StartServer(t, params)
	defer s.Stopper().Stop(ctx)
	_, err := sqlDB.Exec(`
						CREATE DATABASE t;
						CREATE TABLE t.test (k INT8RANGE);
						`)
	require.NoError(t, err)
	_, err = sqlDB.Query(`
						show columns from t.test;
						`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
						insert into t.test (k) values ('(,21)'::int8range);
						`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
						insert into t.test (k) values ('(,)'::int8range);
						`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
						insert into t.test (k) values ('(21,)'::int8range);
						`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
						insert into t.test (k) values (int8range(null,21));
						`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
						insert into t.test (k) values (int8range(null,null));
						`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
						insert into t.test (k) values (int8range(21,null));
						`)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
						insert into t.test (k) values (int8range(21,45));
						`)
	require.NoError(t, err)
	result, err := sqlDB.Query(`
						select * from t.test;
						`)
	require.NoError(t, err)

	var resultOut []map[string]interface{}
	result.Scan(&resultOut)
	bytes, _ := json.Marshal(result)
	fmt.Println(string(bytes))

}
