package tsdb

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/influxdb/influxdb/influxql"
	"github.com/influxdb/influxdb/meta"
)

type Mapper interface {
	// Open will open the necessary resources to being the map job. Could be connections to remote servers or
	// hitting the local store
	Open() error

	// Close will close the mapper
	Close()

	// Begin will set up the Mapper to return series data for the given query.
	Begin(stmt *influxql.SelectStatement, chunkSize int) error

	// NextChunk returns the next chunk of data within the interval, for a specific tag set.
	// interval is a monotonically increasing number based on the group by time and the shard
	// times. It lets the caller know when mappers are processing the same interval
	NextChunk() (tagSet string, result interface{}, interval int, err error)
}

type Planner struct {
	MetaStore interface {
		ShardGroupsByTimeRange(database, policy string, min, max time.Time) (a []meta.ShardGroupInfo, err error)
		NodeID() uint64
	}

	Cluster interface {
		NewMapper(shardID uint64) (Mapper, error)
	}

	Logger *log.Logger
}

func NewPlanner() *Planner {
	return &Planner{
		Logger: log.New(os.Stderr, "[planner] ", log.LstdFlags),
	}
}

// Plan creates an execution plan for the given SelectStatement and returns an Executor.
func (p *Planner) Plan(stmt *influxql.SelectStatement, chunkSize int) (*Executor, error) {
	shards := map[uint64]meta.ShardInfo{} // Shards requiring mappers.

	for _, src := range stmt.Sources {
		mm, ok := src.(*influxql.Measurement)
		if !ok {
			return nil, fmt.Errorf("invalid source type: %#v", src)
		}

		// Replace instances of "now()" with the current time, and check the resultant times.
		stmt.Condition = influxql.Reduce(stmt.Condition, &influxql.NowValuer{Now: time.Now().UTC()})
		tmin, tmax := influxql.TimeRange(stmt.Condition)
		if tmax.IsZero() {
			tmax = time.Now()
		}
		if tmin.IsZero() {
			tmin = time.Unix(0, 0)
		}

		// Build the set of target shards. Using shard IDs as keys ensures each shard ID
		// occurs only once.
		shardGroups, err := p.MetaStore.ShardGroupsByTimeRange(mm.Database, mm.RetentionPolicy, tmin, tmax)
		if err != nil {
			return nil, err
		}
		for _, g := range shardGroups {
			for _, sh := range g.Shards {
				shards[sh.ID] = sh
			}
		}
	}

	// Build the Mappers, one per shard. If the shard is local to this node, always use
	// that one, versus asking the cluster.
	mappers := []Mapper{}
	for _, sh := range shards {
		if sh.OwnedBy(p.MetaStore.NodeID()) {
			mappers = append(mappers, &ShardMapper{})
		} else {
			mapper, err := p.Cluster.NewMapper(sh.ID)
			if err != nil {
				return nil, err
			}
			mappers = append(mappers, mapper)
		}

	}

	return NewExecutor(mappers), nil
}

type Executor struct {
	mappers []Mapper
}

func NewExecutor(mappers []Mapper) *Executor {
	return &Executor{
		mappers: mappers,
	}
}

// Execute begins execution of the query and returns a channel to receive rows.
func (e *Executor) Execute() <-chan *influxql.Row {
	// Create output channel and stream data in a separate goroutine.
	out := make(chan *influxql.Row, 0)

	return out
}

type ShardMapper struct {
}

func (sm *ShardMapper) Open() error {
	return nil
}

func (sm *ShardMapper) Close() {
}

func (sm *ShardMapper) Begin(stmt *influxql.SelectStatement, chunkSize int) error {
	return nil
}

func (sm *ShardMapper) NextChunk() (tagSet string, result interface{}, interval int, err error) {
	return "", nil, 0, nil
}
