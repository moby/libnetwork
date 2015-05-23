.PHONY: all all-local build build-local check check-code check-format run-tests check-local install-deps coveralls circle-ci
SHELL=/bin/bash
build_image=libnetwork-build
dockerargs = --privileged -v $(shell pwd):/go/src/github.com/docker/libnetwork -w /go/src/github.com/docker/libnetwork
container_env = -e "INSIDECONTAINER=-incontainer=true"
docker = docker run --rm ${dockerargs} ${container_env} ${build_image}
ciargs = -e "COVERALLS_TOKEN=$$COVERALLS_TOKEN" -e "INSIDECONTAINER=-incontainer=true"
cidocker = docker run ${ciargs} ${dockerargs} golang:1.4

#Check if a new build image needs to be created
#Return value 1 : Build image must be created.
#Build image doesn't exist or Build image exist but Makefile has changed from previous build
#Return value 0 : No need to create a new build image.
#Build image exist and Makefile hasn't changed from previous build
LIBNETWORK_BUILD_DATE=$(shell docker inspect ${build_image}|grep 'Created'|grep -Po '".*?"'|grep -v 'Created' | sed -e 's/^"//'  -e 's/"//')
ifeq ($(LIBNETWORK_BUILD_DATE),)
#libnetwork-build doesn't exist
#Set flag to create libnetwork-build image
createbuildimage=1
else
#libnetwork-build exists
#Check if Makefile is newer than libnetwork-build
MAKEFILE_MODIFY_DATE=$(shell stat Makefile|grep Modify|cut -c9-)
LIBNETWORK_EPOCH=$(shell date --date="${LIBNETWORK_BUILD_DATE}" +"%s")
MAKEFILE_EPOCH=$(shell date --date="${MAKEFILE_MODIFY_DATE}" +"%s")
ifeq ($(shell if [ ${LIBNETWORK_EPOCH} -lt ${MAKEFILE_EPOCH} ]; then echo lt; else echo gt; fi),lt)
#libnetwork-build older than Makefile need to create new image in case
#install-deps have been modified or because flags have changed of for any other reason
#Set flag to create libnetwork-build image
createbuildimage=1
else
#libnetwork-build newer than Makefile we don't need to create new image
#Set flag to avoid creating a new libnetwork-build image
createbuildimage=0
endif
endif

ifeq ($(createbuildimage), 0)
$(shell touch ${build_image}.created)
else
$(shell rm ${build_image}.created)
endif


all:${build_image}.created
	${docker} make all-local

all-local: check-local build-local

${build_image}.created:
	docker run --name=libnetworkbuild -v $(shell pwd):/go/src/github.com/docker/libnetwork -w /go/src/github.com/docker/libnetwork golang:1.4 make install-deps
	docker commit libnetworkbuild ${build_image}
	docker rm libnetworkbuild
	touch ${build_image}.created

build: ${build_image}.created
	${docker} make build-local

build-local:
	$(shell which godep) go build ./...

check: ${build_image}.created
	${docker} make check-local

check-code:
	@echo "Checking code... "
	test -z "$$(golint ./... | tee /dev/stderr)"
	go vet ./...
	@echo "Done checking code"

check-format:
	@echo "Checking format... "
	test -z "$$(goimports -l . | grep -v Godeps/_workspace/src/ | tee /dev/stderr)"
	@echo "Done checking format"

run-tests:
	@echo "Running tests... "
	@echo "mode: count" > coverage.coverprofile
	@for dir in $$(find . -maxdepth 10 -not -path './.git*' -not -path '*/_*' -type d); do \
	    if ls $$dir/*.go &> /dev/null; then \
		pushd . &> /dev/null ; \
		cd $$dir ; \
		$(shell which godep) go test ${INSIDECONTAINER} -test.parallel 3 -test.v -covermode=count -coverprofile=./profile.tmp ; \
		ret=$$? ;\
		if [ $$ret -ne 0 ]; then exit $$ret; fi ;\
		popd &> /dev/null; \
	        if [ -f $$dir/profile.tmp ]; then \
		        cat $$dir/profile.tmp | tail -n +2 >> coverage.coverprofile ; \
				rm $$dir/profile.tmp ; \
            fi ; \
        fi ; \
	done
	@echo "Done running tests"

check-local: 	check-format check-code run-tests 

install-deps:
	apt-get update && apt-get -y install iptables
	go get github.com/tools/godep
	go get github.com/golang/lint/golint
	go get golang.org/x/tools/cmd/vet
	go get golang.org/x/tools/cmd/goimports
	go get golang.org/x/tools/cmd/cover
	go get github.com/mattn/goveralls

coveralls:
	-@goveralls -service circleci -coverprofile=coverage.coverprofile -repotoken $$COVERALLS_TOKEN

# CircleCI's Docker fails when cleaning up using the --rm flag
# The following target is a workaround for this

circle-ci:
	@${cidocker} make install-deps check-local coveralls
