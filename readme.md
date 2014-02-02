# Docker Cluster
This is experimental cluster support for Docker.  It uses Raft for communication between Docker hosts.

# Usage
This shows an example between two Docker hosts.

Host 1:
`docker-cluster -h 10.1.1.10 /tmp/docker-cluster`

Host 2:
`docker-cluster -h 10.1.1.20 -join 10.1.1.10:4500 /tmp/docker-cluster`

Now you will be able to use the standard Docker client with any of the nodes in the cluster:

`docker -H tcp://10.1.1.10:4500 ps`

`docker -H tcp://10.1.1.20:4500 ps`

# Status
Currently only viewing hosts is supported (`docker ps` and `docker ps -a`).  Support for all operations will continuously added.
