from ubuntu:12.04
maintainer Evan Hazlett <ejhazlett@gmail.com> @ehazlett
run apt-get install -y libdevmapper btrfs-tools libsqlite3-0
add ./docker-hive /usr/local/bin/docker-hive
expose 4500
entrypoint ["/usr/local/bin/docker-hive"]
cmd ["-docker", "/docker.sock", "/tmp/hive"]
