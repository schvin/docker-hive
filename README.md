# Docker Hive [![Build Status](https://ejh-drone.arcus.io/github.com/ehazlett/docker-hive/status.png?branch=master)](https://ejh-drone.arcus.io/github.com/ehazlett/docker-hive)
This is experimental cluster support for Docker.  It uses Raft for communication between Docker hosts.

# Usage
This shows an example between two Docker hosts.

Host 1:
`docker-hive -n 10.1.1.10 /tmp/docker-hive`

Host 2:
`docker-hive -n 10.1.1.20 -join 10.1.1.10:4500 /tmp/docker-hive`

Now you will be able to use the standard Docker client with any of the nodes in the cluster:

* `docker -H tcp://10.1.1.10:4500 ps`
* `docker -H tcp://10.1.1.10:4500 ps -a`
* `docker -H tcp://10.1.1.10:4500 images`
* `docker -H tcp://10.1.1.20:4500 ps`

# Status
Very early development.  Not for production.
