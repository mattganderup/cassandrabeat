package beater

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/publisher"

	"github.com/goomzee/cassandrabeat/config"
)

type Cassandrabeat struct {
	done   chan struct{}
	config config.Config
	client publisher.Client

	table []string
}

// Creates beater
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	config := config.DefaultConfig
	if err := cfg.Unpack(&config); err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}

	bt := &Cassandrabeat{
		done:   make(chan struct{}),
		config: config,
	}
	return bt, nil
}

func (bt *Cassandrabeat) Run(b *beat.Beat) error {
	logp.Info("cassandrabeat is running! Hit CTRL-C to stop it.")

	bt.client = b.Publisher.Connect()
	bt.table = bt.config.Table[:]
	ticker := time.NewTicker(bt.config.Period)
	for {
		select {
		case <-bt.done:
			return nil
		case <-ticker.C:
		}

		for _, table := range bt.table {
			logp.Info("Getting latency for table: %s", table)
			bt.getMetrics(table)
		}
		logp.Info("Event sent")
	}
}

func (bt *Cassandrabeat) Stop() {
	bt.client.Close()
	close(bt.done)
}

func (bt *Cassandrabeat) getMetrics(table string) {
	cmdName := "awkscript.sh"
	cmdArgs := []string{table}
	cmdOut := exec.Command(cmdName, cmdArgs...).Output

	output, err := cmdOut()
	if err != nil {
		fmt.Println(err)
		time.Sleep(5000 * time.Millisecond)
		return
	}

	metrics := strings.Split(string(output), "\n")

	/*
		fmt.Printf("Results back from nodetool cfstats for %s\n", table)
		for i := range latency {
			fmt.Println(i, latency[i])
		}
	*/

	if len(metrics) < 8 {
		fmt.Printf("Not enough values (%v) returned from nodetool script. Bailing.\n", len(metrics))
		return
	}

	var readLatency, writeLatency float64
	var pendingFlushes, sstableCount, spaceUsedLive, spaceUsedTotal int64
	var spaceUsedSnapshotTotal, numberOfKeys int64

	if strings.Compare(metrics[0], "NAN") == 0 {
		pendingFlushes = 0
	} else {
		pendingFlushes, _ = strconv.ParseInt(metrics[0], 10, 64)
	}
	if strings.Compare(metrics[1], "NAN") == 0 {
		sstableCount, _ = strconv.ParseInt(metrics[1], 10, 64)
	} else {
		sstableCount = 0
	}
	if strings.Compare(metrics[2], "NAN") == 0 {
		spaceUsedLive, _ = strconv.ParseInt(metrics[2], 10, 64)
	} else {
		spaceUsedLive = 0
	}
	if strings.Compare(metrics[3], "NAN") == 0 {
		spaceUsedTotal, _ = strconv.ParseInt(metrics[3], 10, 64)
	} else {
		spaceUsedTotal = 0
	}
	if strings.Compare(metrics[4], "NAN") == 0 {
		spaceUsedSnapshotTotal, _ = strconv.ParseInt(metrics[4], 10, 64)
	} else {
		spaceUsedSnapshotTotal = 0
	}
	if strings.Compare(metrics[5], "NAN") == 0 {
		numberOfKeys, _ = strconv.ParseInt(metrics[5], 10, 64)
	} else {
		numberOfKeys = 0
	}
	if strings.Compare(metrics[6], "NaN") == 0 {
		readLatency = 0.0
	} else {
		readLatency, _ = strconv.ParseFloat(metrics[6], 64)
	}
	if strings.Compare(metrics[7], "NaN") == 0 {
		writeLatency = 0.0
	} else {
		writeLatency, _ = strconv.ParseFloat(metrics[7], 64)
	}

	event := common.MapStr{
		"@timestamp":                common.Time(time.Now()),
		"type":                      "stats",
		"count":                     1,
		"table_name":                table,
		"write_latency":             writeLatency,
		"read_latency":              readLatency,
		"pending_flushes":           pendingFlushes,
		"sstable_count":             sstableCount,
		"space_used_live":           spaceUsedLive,
		"space_used_total":          spaceUsedTotal,
		"space_used_snapshot_total": spaceUsedSnapshotTotal,
		"number_of_keys":            numberOfKeys,
	}

	bt.client.PublishEvent(event)
}
