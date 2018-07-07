# Distributed Fizz Buzz

## What does it do?

One process takes a lock on etcd and gets to print that number, fizz or buzz.

The other processes can wait and compete for a lock for the next number.
