package postgres

import (
	"database/sql"
	"reflect"
	"strings"

	"github.com/perrito666/bmstrem/db/logging"
	"github.com/perrito666/bmstrem/db/srm"
	"github.com/pkg/errors"

	"github.com/jackc/pgx"
	"github.com/perrito666/bmstrem/db/connection"
)

var _ connection.DatabaseHandler = &Connector{}
var _ connection.DB = &DB{}

// Connector implements connection.Handler
type Connector struct {
	ConnectionString string
}

// DefaultPGPoolMaxConn is an arbitrary number of connections that I decided was ok for the pool
const DefaultPGPoolMaxConn = 10

// Open opens a connection to postgres and returns it wrapped into a connection.DB
func (c *Connector) Open(ci *connection.Information) (connection.DB, error) {
	// Ill be opinionated here and use the most efficient params.
	config := pgx.ConnPoolConfig{
		ConnConfig: pgx.ConnConfig{
			Host:     ci.Host,
			Port:     ci.Port,
			Database: ci.Database,
			User:     ci.User,
			Password: ci.Password,

			TLSConfig:         ci.TLSConfig,
			UseFallbackTLS:    ci.UseFallbackTLS,
			FallbackTLSConfig: ci.FallbackTLSConfig,
			Logger:            logging.NewPgxLogAdapter(ci.Logger),
		},
		MaxConnections: ci.MaxConnPoolConns,
	}
	conn, err := pgx.NewConnPool(config)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to postgres database")
	}
	return &DB{
		conn:   conn,
		logger: ci.Logger,
	}, nil
}

// DB wraps pgx.Conn into a struct that implements connection.DB
type DB struct {
	conn   *pgx.ConnPool
	tx     *pgx.Tx
	logger logging.Logger
}

// Clone returns a copy of DB with the same underlying Connection
func (d *DB) Clone() connection.DB {
	return &DB{
		conn:   d.conn,
		logger: d.logger,
	}
}

func snakesToCamels(s string) string {
	var c string
	var snake bool
	for i, v := range s {
		if i == 0 {
			c += strings.ToUpper(string(v))
			continue
		}
		if v == '_' {
			snake = true
			continue
		}
		if snake {
			c += strings.ToUpper(string(v))
			continue
		}
		c += string(v)
	}
	return c
}

// QueryIter returns an iterator that can be used to fetch results one by one, beware this holds
// the connection until fetching is done.
// the passed fields are supposed to correspond to the fields being brought from the db, no
// check is performed on this.
func (d *DB) QueryIter(statement string, fields []string, args ...interface{}) (connection.ResultFetchIter, error) {
	var rows *pgx.Rows
	var err error
	if d.conn != nil {
		rows, err = d.conn.Query(statement, args...)
	} else {
		// yes, this is a leap of fait that one is set
		rows, err = d.tx.Query(statement, args...)
	}
	if err != nil {
		return func(interface{}) (bool, func(), error) { return false, func() {}, nil },
			errors.Wrap(err, "querying database")
	}

	var fieldMap map[string]reflect.StructField
	var typeName string
	if !rows.Next() {
		return func(interface{}) (bool, func(), error) { return false, func() {}, nil },
			sql.ErrNoRows
	}
	if len(fields) == 0 {
		// This seems to make a query each time so perhaps it goes outside.
		sqlQueryfields := rows.FieldDescriptions()
		fields = make([]string, len(sqlQueryfields), len(sqlQueryfields))
		for i, v := range sqlQueryfields {
			fields[i] = v.Name
		}
	}
	return func(destination interface{}) (bool, func(), error) {
		var err error
		if reflect.TypeOf(destination).Name() != typeName {
			typeName, fieldMap, err = srm.MapFromPtrType(destination, []reflect.Kind{}, []reflect.Kind{
				reflect.Map, reflect.Slice,
			})
			if err != nil {
				defer rows.Close()
				return false, func() {}, errors.Wrapf(err, "cant fetch data into %T", destination)
			}
		}
		fieldRecipients := srm.FieldRecipientsFromType(fields, fieldMap, destination)

		err = rows.Scan(fieldRecipients...)
		if err != nil {
			defer rows.Close()
			return false, func() {}, errors.Wrap(err, "scanning values into recipient, connection was closed")
		}

		return rows.Next(), rows.Close, nil
	}, nil
}

// Query returns a function that allows recovering the results of the query, beware the connection
// is held until the returned closusure is invoked.
func (d *DB) Query(statement string, fields []string, args ...interface{}) (connection.ResultFetch, error) {
	var rows *pgx.Rows
	var err error
	if d.conn != nil {
		rows, err = d.conn.Query(statement, args...)
	} else {
		// yes, this is a leap of fait that one is set
		rows, err = d.tx.Query(statement, args...)
	}
	if err != nil {
		return func(interface{}) error { return nil },
			errors.Wrap(err, "querying database")
	}
	var fieldMap map[string]reflect.StructField
	var typeName string
	return func(destination interface{}) error {
		// TODO add a timer that closes rows if nothing is done.
		defer rows.Close()
		var err error
		typeName, fieldMap, err = srm.MapFromPtrType(destination, []reflect.Kind{reflect.Slice}, []reflect.Kind{})
		if err != nil {
			defer rows.Close()
			return errors.Wrapf(err, "cant fetch data into %T", destination)
		}

		// Obtain the actual slice
		destinationSlice := reflect.ValueOf(destination).Elem()

		// If this is not Ptr->Slice->Type it would have failed already.
		tod := reflect.TypeOf(destination).Elem().Elem()

		if len(fields) == 0 {
			// This seems to make a query each time so perhaps it goes outside.
			sqlQueryfields := rows.FieldDescriptions()
			fields = make([]string, len(sqlQueryfields), len(sqlQueryfields))
			for i, v := range sqlQueryfields {
				fields[i] = v.Name
			}
		}
		for rows.Next() {
			// Get a New object of the type of the slice.
			newElem := reflect.Zero(tod)
			if typeName != tod.Name() {
				typeName = tod.Name()
				fieldMap = make(map[string]reflect.StructField, tod.NumField())
				for fieldIndex := 0; fieldIndex > tod.NumField(); fieldIndex++ {
					field := tod.Field(fieldIndex)
					fieldMap[field.Name] = field
				}
			}
			// Construct the recipient fields.
			fieldRecipients := srm.FieldRecipientsFromType(fields, fieldMap, newElem.Addr())

			// Try to fetch the data
			err = rows.Scan(fieldRecipients...)
			if err != nil {
				defer rows.Close()
				return errors.Wrap(err, "scanning values into recipient, connection was closed")
			}
			// Add to the passed slice, this will actually add to an already populated slice if one
			// passed, how cool is that?
			destinationSlice.Set(reflect.Append(destinationSlice, newElem))
		}
		return nil
	}, nil
}

// Raw will run the passed statement with the passed args and scan the first resul, if any,
// to the passed fields.
func (d *DB) Raw(statement string, args []interface{}, fields ...interface{}) error {
	var rows *pgx.Row
	if d.conn != nil {
		rows = d.conn.QueryRow(statement, args...)
	} else {
		// yes, this is a leap of fait that one is set
		rows = d.tx.QueryRow(statement, args...)
	}

	// Try to fetch the data
	err := rows.Scan(fields...)
	if err != nil {
		return errors.Wrap(err, "scanning values into recipient")
	}
	return nil
}

// Exec will run the statement and expect nothing in return.
func (d *DB) Exec(statement string, args ...interface{}) error {
	var connTag pgx.CommandTag
	var err error
	if d.conn != nil {
		connTag, err = d.conn.Exec(statement, args...)
	} else {
		// yes, this is a leap of fait that one is set
		connTag, err = d.tx.Exec(statement, args...)
	}
	if err != nil {
		return errors.Wrapf(err, "querying database, obtained %s", connTag)
	}
	return nil
}

// BeginTransaction returns a new DB that will use the transaction instead of the basic conn.
// if the transaction is already started the same will be returned.
func (d *DB) BeginTransaction() (connection.DB, error) {
	if d.tx != nil {
		return d, nil
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, errors.Wrap(err, "trying to begin a transaction")
	}
	return &DB{
		tx:     tx,
		logger: d.logger,
	}, nil
}

// IsTransaction indicates if the DB is in the middle of a transaction.
func (d *DB) IsTransaction() bool {
	return d.tx != nil
}

// CommitTransaction commits the transaction if any is in course, beavior comes straight from
// pgx.
func (d *DB) CommitTransaction() error {
	if d.tx == nil {
		return nil
	}
	return d.tx.Commit()
}

// RollbackTransaction rolls back the transaction if any is in course, beavior comes straight from
// pgx.
func (d *DB) RollbackTransaction() error {
	if d.tx == nil {
		return nil
	}
	return d.tx.Rollback()
}

// Set tries to run `SET LOCAL` with the passed parameters if there is an ongoing transaction.
func (d *DB) Set(set string) error {
	if d.tx == nil {
		return nil
	}
	// TODO check if this will work in the `SET LOCAL $1` arg format
	cTag, err := d.tx.Exec("SET LOCAL " + set)
	if err != nil {
		return errors.Wrapf(err, "trying to set local, returned: %s", cTag)
	}
	return nil
}
