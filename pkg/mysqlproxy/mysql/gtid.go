// Copyright 2022-present The ZTDBP Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mysql

import "github.com/pingcap/errors"

type GTIDSet interface {
	String() string

	// Encode GTID set into binary format used in binlog dump commands
	Encode() []byte

	Equal(o GTIDSet) bool

	Contain(o GTIDSet) bool

	Update(GTIDStr string) error

	Clone() GTIDSet
}

func ParseGTIDSet(flavor string, s string) (GTIDSet, error) {
	switch flavor {
	case MySQLFlavor:
		return ParseMysqlGTIDSet(s)
	case MariaDBFlavor:
		return ParseMariadbGTIDSet(s)
	default:
		return nil, errors.Errorf("invalid flavor %s", flavor)
	}
}
