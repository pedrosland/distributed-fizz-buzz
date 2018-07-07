package main

import (
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"context"
	"time"
	"fmt"
	"log"
	"flag"
	"os"
	"os/signal"
	"strconv"
)

const (
	dialTimeout    = 2 * time.Second
	requestTimeout = 10 * time.Second
)

var reset = flag.Bool("reset", false, "Reset the counter")
var maxValue = flag.Int("maxval", 20, "Maximum counter value")

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	cli, _ := clientv3.New(clientv3.Config{
		DialTimeout: dialTimeout,
		Endpoints: []string{"127.0.0.1:2379"},
	})
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

		gr, err := kv.Get(ctx, "/counter/current")
		if err != nil {
			if err == context.Canceled {
				return
			}
			log.Fatalf("error getting current counter value: %s", err)
		}

		currentValue := 0
		if len(gr.Kvs) > 0 {
			// Note: this would overflow but it is constrained by the maxval flag
			currentValue, err = strconv.Atoi(string(gr.Kvs[0].Value))
			if err != nil {
				// should not happen
				log.Fatalf("invalid value for counter in etcd: could not conver to int: %s", err)
			}
		}

		if currentValue >= *maxValue {
			m.Unlock(ctx)
			return
		}

		currentValue++
		printFizzBuzz(currentValue)
		_, err = kv.Put(ctx, "/counter/current", strconv.Itoa(currentValue))
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
	fizz := num % 3 == 0
	buzz := num % 5 == 0

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

// Deletes the counter key to reset it
func resetEtcd(ctx context.Context, kv clientv3.KV) {
	ctx, _ = context.WithTimeout(ctx, requestTimeout)
	_, err := kv.Delete(ctx, "/counter/current")
	if err != nil {
		log.Fatalf("error resetting current counter value: %s", err)
	}
}
