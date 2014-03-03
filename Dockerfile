from stackbrew/debian:jessie
maintainer Evan Hazlett <ejhazlett@gmail.com> @ehazlett
add ./docker-hive /usr/local/bin/docker-hive
expose 4500
entrypoint ["/usr/local/bin/docker-hive"]
cmd ["-docker", "/docker.sock", "/tmp/hive"]
