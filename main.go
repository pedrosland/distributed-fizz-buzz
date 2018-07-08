package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"time"
)

const (
	dialTimeout    = 2 * time.Second
	requestTimeout = 10 * time.Second
)

var reset = flag.Bool("reset", false, "Reset the counter")
var bench = flag.Bool("bench", false, "Runs basic benchmark")
var pprof = flag.Bool("pprof", false, "Enable pprof")
var maxValue = flag.Int("maxval", 20, "Maximum counter value")

func main() {
	flag.Parse()

	if *pprof {
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	ctx, cancel := context.WithCancel(context.Background())
	cli, err := clientv3.New(clientv3.Config{
		DialTimeout: dialTimeout,
		Endpoints:   []string{"127.0.0.1:2379"},
	})
	if err != nil {
		log.Fatalf("error connecting to etcd: %s", err)
	}
	defer cli.Close()
	kv := clientv3.NewKV(cli)
	session, err := concurrency.NewSession(cli)
	if err != nil {
		log.Fatalf("error creating new session: %s", err)
	}
	defer session.Close()

	if *reset {
		resetEtcd(ctx, kv)
		log.Print("etcd counter reset")
	}

	if *bench {
		runBench(ctx, kv, session)
		return
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	done := make(chan bool)
	go StartFizzBuzzer(ctx, kv, session, done)

	stopping := false
	go func() {
		<-c
		if stopping {
			log.Fatal("Force exiting")
			return
		}
		stopping = true
		log.Print("Received signal. Stopping FizzBuzzer...")
		cancel()
	}()

	<-done
	log.Print("Done")
}

func StartFizzBuzzer(parentCtx context.Context, kv clientv3.KV, session *concurrency.Session, done chan bool) {
	defer func() { done <- true }()
	var err error
	currentValue := 0
	m := concurrency.NewMutex(session, "/counter/lock")

	for {
		ctx, _ := context.WithTimeout(parentCtx, requestTimeout)
		if err := m.Lock(ctx); err != nil {
			if err == context.Canceled {
				return
			}
			if err == context.DeadlineExceeded {
				continue
			}
			log.Fatalf("error getting lock: %s", err)
		}

		currentValue, err = getValue(ctx, kv)
		if err == context.Canceled {
			return
		}

		if currentValue >= *maxValue {
			m.Unlock(ctx)
			return
		}

		currentValue++
		printFizzBuzz(currentValue)
		_, err := kv.Put(ctx, "/counter/current", strconv.Itoa(currentValue))
		if err != nil {
			if err == context.Canceled {
				return
			}
			log.Fatalf("error updating counter value: %s", err)
		}

		//time.Sleep(1 * time.Second)

		// defer not required as it will release with the session if there is an error
		m.Unlock(ctx)
	}
}

func printFizzBuzz(num int) {
	fizz := num%3 == 0
	buzz := num%5 == 0

	if fizz && buzz {
		fmt.Println("FizzBuzz")
	} else if fizz {
		fmt.Println("Fizz")
	} else if buzz {
		fmt.Println("Buzz")
	} else {
		fmt.Println(num)
	}
}

func getValue(ctx context.Context, kv clientv3.KV) (int, error) {
	value := 0
	gr, err := kv.Get(ctx, "/counter/current")
	if err != nil {
		if err == context.Canceled {
			return 0, err
		}
		log.Fatalf("error getting current counter value: %s", err)
	}

	if len(gr.Kvs) > 0 {
		// Note: this would overflow but it is constrained by the maxval flag
		value, err = strconv.Atoi(string(gr.Kvs[0].Value))
		if err != nil {
			// should not happen
			log.Fatalf("invalid value for counter in etcd: could not conver to int: %s", err)
		}
	}

	return value, nil
}

// resetEtcd deletes the counter key to reset it
func resetEtcd(ctx context.Context, kv clientv3.KV) {
	ctx, _ = context.WithTimeout(ctx, requestTimeout)
	_, err := kv.Delete(ctx, "/counter/current")
	if err != nil {
		log.Fatalf("error resetting current counter value: %s", err)
	}
}

// runBench runs a particularly untrustworthy benchmark
func runBench(ctx context.Context, kv clientv3.KV, session *concurrency.Session) {
	done := make(chan bool, 1)
	resetEtcd(ctx, kv)

	start := time.Now()
	StartFizzBuzzer(ctx, kv, session, done)
	diff := time.Now().Sub(start)

	value, _ := getValue(ctx, kv)

	fmt.Printf("Benchmark completed. Reached %d in %0.3f seconds\n", value, float64(diff.Nanoseconds())/float64(time.Second))

	time.Sleep(10 * time.Second) // enough time for the profiler to finish
}
