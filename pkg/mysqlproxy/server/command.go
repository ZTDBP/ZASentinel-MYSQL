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

package server

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/siddontang/go/hack"

	. "github.com/ztalab/ZASentinel-MYSQL/pkg/mysqlproxy/mysql"
)

type Handler interface {
	// client close
	CloseConn() error
	// handle COM_INIT_DB command, you can check whether the dbName is valid, or other.
	UseDB(dbName string) error
	// handle COM_QUERY command, like SELECT, INSERT, UPDATE, etc...
	// If Result has a Resultset (SELECT, SHOW, etc...), we will send this as the response, otherwise, we will send Result
	HandleQuery(query string) (*Result, error)
	// handle COM_FILED_LIST command
	HandleFieldList(table string, fieldWildcard string) ([]*Field, error)
	// handle COM_STMT_PREPARE, params is the param number for this statement, columns is the column number
	// context will be used later for statement execute
	HandleStmtPrepare(query string) (params int, columns int, context interface{}, err error)
	// handle COM_STMT_EXECUTE, context is the previous one set in prepare
	// query is the statement prepare query, and args is the params for this statement
	HandleStmtExecute(context interface{}, query string, args []interface{}) (*Result, error)
	// handle COM_STMT_CLOSE, context is the previous one set in prepare
	// this handler has no response
	HandleStmtClose(context interface{}) error
	// handle any other command that is not currently handled by the library,
	// default implementation for this method will return an ER_UNKNOWN_ERROR
	HandleOtherCommand(cmd byte, data []byte) error
}

func (c *Conn) HandleCommand(connErrHandle func(*Conn, error, uint8) error, otherErrHandle func(*Conn, error)) (clientError error) {
	if c.Conn == nil {
		return fmt.Errorf("connection closed")
	}
	// read the request from the client
	data, err := c.ReadPacket()
	if err != nil {
		c.Close()
		c.Conn = nil
		return nil
	}
	// After processing the client request data,
	// forward it to the target mysql service and receive the mysql service response
	var v interface{}
	var connErr error
	// The agent sends data to the remote end, and if there is an error,
	// decides whether to retry based on the error function
	var retry uint8
	for {
		v = c.dispatch(data)
		if e, ok := v.(error); ok {
			if !errors.Is(e, ErrBadConn) {
				otherErrHandle(c, e)
				break
			}
			// 只处理连接错误，当重新分配连接时重试往远端mysql发送请求
			// 当连接分配重试次数大于一次时，返回错误结束
			connErr = connErrHandle(c, e, retry)
			if connErr != nil {
				break
			}
			retry++
		} else {
			break
		}
	}

	// 把目标mysql服务响应的数据写入客户端
	writeErr := c.writeValue(v)

	if c.Conn != nil {
		c.ResetSequence()
	}

	if writeErr != nil || connErr != nil {
		c.Close()
		c.Conn = nil
	}
	return err
}

/*
func (c *Conn) HandleCommand() error {
	if c.Conn == nil {
		return fmt.Errorf("connection closed")
	}

	data, err := c.ReadPacket()
	if err != nil {
		c.Close()
		c.Conn = nil
		return err
	}

	v := c.dispatch(data)

	err = c.writeValue(v)

	if c.Conn != nil {
		c.ResetSequence()
	}

	if err != nil {
		c.Close()
		c.Conn = nil
	}
	return err
}
*/
func (c *Conn) dispatch(data []byte) interface{} {
	cmd := data[0]
	data = data[1:]

	switch cmd {
	case COM_QUIT:
		_ = c.h.CloseConn()
		c.Close()
		c.Conn = nil
		return noResponse{}
	case COM_QUERY:
		if r, err := c.h.HandleQuery(hack.String(data)); err != nil {
			return err
		} else {
			return r
		}
	case COM_PING:
		return nil
	case COM_INIT_DB:
		if err := c.h.UseDB(hack.String(data)); err != nil {
			return err
		} else {
			return nil
		}
	case COM_FIELD_LIST:
		index := bytes.IndexByte(data, 0x00)
		table := hack.String(data[0:index])
		wildcard := hack.String(data[index+1:])

		if fs, err := c.h.HandleFieldList(table, wildcard); err != nil {
			return err
		} else {
			return fs
		}
	case COM_STMT_PREPARE:
		c.stmtID++
		st := new(Stmt)
		st.ID = c.stmtID
		st.Query = hack.String(data)
		var err error
		if st.Params, st.Columns, st.Context, err = c.h.HandleStmtPrepare(st.Query); err != nil {
			return err
		} else {
			st.ResetParams()
			c.stmts[c.stmtID] = st
			return st
		}
	case COM_STMT_EXECUTE:
		if r, err := c.handleStmtExecute(data); err != nil {
			return err
		} else {
			return r
		}
	case COM_STMT_CLOSE:
		if err := c.handleStmtClose(data); err != nil {
			return err
		}
		return noResponse{}
	case COM_STMT_SEND_LONG_DATA:
		if err := c.handleStmtSendLongData(data); err != nil {
			return err
		}
		return noResponse{}
	case COM_STMT_RESET:
		if r, err := c.handleStmtReset(data); err != nil {
			return err
		} else {
			return r
		}
	case COM_SET_OPTION:
		if err := c.h.HandleOtherCommand(cmd, data); err != nil {
			return err
		}

		return eofResponse{}
	default:
		return c.h.HandleOtherCommand(cmd, data)
	}

	return fmt.Errorf("command %d is not handled correctly", cmd)
}

type EmptyHandler struct{}

func (h EmptyHandler) CloseConn() error {
	return nil
}

func (h EmptyHandler) UseDB(dbName string) error {
	return nil
}

func (h EmptyHandler) HandleQuery(query string) (*Result, error) {
	return nil, fmt.Errorf("not supported now")
}

func (h EmptyHandler) HandleFieldList(table string, fieldWildcard string) ([]*Field, error) {
	return nil, fmt.Errorf("not supported now")
}

func (h EmptyHandler) HandleStmtPrepare(query string) (int, int, interface{}, error) {
	return 0, 0, nil, fmt.Errorf("not supported now")
}

func (h EmptyHandler) HandleStmtExecute(context interface{}, query string, args []interface{}) (*Result, error) {
	return nil, fmt.Errorf("not supported now")
}

func (h EmptyHandler) HandleStmtClose(context interface{}) error {
	return nil
}

func (h EmptyHandler) HandleOtherCommand(cmd byte, data []byte) error {
	return NewError(
		ER_UNKNOWN_ERROR,
		fmt.Sprintf("command %d is not supported now", cmd),
	)
}
