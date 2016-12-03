libnetwork proposals
====================

This is a working document for proposals that will be made to Docker Engine.

## Replace Networking Subsystem with libnetwork

< todo >

## Add "Network" as a first class object in Docker

### CLI Changes


| Command | Flags | Arguments | Example |
|---------|-------|---------|---------|
| docker network create | -d, --driver <br> -n, --name | | docker network create -d overlay --name=foo |
| docker network ls | | | docker network ls |
| docker network info | | required: network name | docker network info foo |
| docker network rm | | required: name | docker-network rm foo |
| docker network join | | required: container id or name <br> required: network name | docker network join aabbccdd1122 foo |
| docker network leave | | required: container id or name <br> required: network name | docker network leave aabbccdd1122 foo |
| docker network endpoint ls | | | docker network endpoint ls |
| docker network endpoint info | | required: endpoint id | docker network endpoint info veth01234567 |

### Remote API changes

< todo >

