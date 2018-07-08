# Distributed Fizz Buzz

This is a silly project that came from a joke about recruiting developers.

## What does it do?

One process takes a lock on etcd and gets to print that number, fizz or buzz.

The other processes can wait and compete for a lock for the next number.

New fizz-buzzers can start any time and will automatically join in. They will not start from 0 but continue counting.

## Running

```
docker-compose up -d
go build
./distributed-fizz-buzz -maxval 1000
```

To run multiple and see their results on the same terminal try:

```
(
  ./distributed-fizz-buzz -maxval 100
  ./distributed-fizz-buzz -maxval 100
)
```

After you've run some numbers, but you want more, you can reset the counter. Just add the `-reset` flag.
It can also be combined with `-maxval 0`.

```
./distributed-fizz-buzz -reset -maxval 0
```
