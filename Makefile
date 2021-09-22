# special makefile variables
.DEFAULT_GOAL := help
.RECIPEPREFIX := >

# recursive variables
SHELL = /usr/bin/sh
BUILD_DIR = ./build
# TODO(cavcrosby): have the target executable also be generated in the BUILD_DIR.
# Make sure to account for ci.
TARGET_EXEC = debcomprt
UPSTREAM_TARBALL_EXT = .orig.tar.gz

# DISCUSS(cavcrosby): according to the make manual, every makefile should define
# the INSTALL variable. Perhaps look into this later? How does this affect
# previous makefiles?
# executables
GO = go
GIT = git
ADDLICENSE = addlicense
executables = \
	${GIT}\
	${GO}

# tools, inspired by:
# https://stackoverflow.com/questions/56636580/replace-retool-with-tools-go-for-multi-developer-and-ci-environments-using-go-mo#answer-56640587
GO_TOOLS = github.com/google/addlicense

# gnu install directory variables, for reference:
# https://golang.org/doc/tutorial/compile-install
prefix = $(shell if [ -n "${GOBIN}" ]; then echo "${GOBIN}"; else echo "${GOPATH}/bin"; fi)
exec_prefix = ${prefix}
bin_dir = ${exec_prefix}/bin

# targets
HELP = help
INSTALL = install
INSTALL_TOOLS = install-tools
TEST = test
ADD_LICENSE = add-license
UPSTREAM_TARBALL = upstream-tarball
DEB = deb
CLEAN = clean

# to be passed in at make runtime
COPYRIGHT_HOLDERS =

# simply expanded variables
# inspired from:
# https://devconnected.com/how-to-list-git-tags/#Find_Latest_Git_Tag_Available
version = $(shell ${GIT} describe --tags --abbrev=0 | sed 's/v//')
src := $(shell find . \( -type f \) -and \( -iname '*.go' \) -and \( -not -iregex '.*/vendor.*' \))
_upstream_tarball_prefix = ${TARGET_EXEC}-${version}
_upstream_tarball = ${_upstream_tarball_prefix}${UPSTREAM_TARBALL_EXT}
_upstream_tarball_dash_to_underscore = $(shell echo "${_upstream_tarball}" | awk --field-separator='-' '{print $$1"_"$$2}')
_upstream_tarball_path = ${BUILD_DIR}/${_upstream_tarball}

# inspired from:
# https://stackoverflow.com/questions/5618615/check-if-a-program-exists-from-a-makefile#answer-25668869
_check_executables := $(foreach exec,${executables},$(if $(shell command -v ${exec}),pass,$(error "No ${exec} in PATH")))

.PHONY: ${HELP}
${HELP}:
	# inspired by the makefiles of the Linux kernel and Mercurial
>	@echo 'Common make targets:'
>	@echo '  ${TARGET_EXEC}          - the ${TARGET_EXEC} binary'
>	@echo '  ${INSTALL}            - installs the local decomprt binary (pathing: ${prefix})'
>	@echo '  ${INSTALL_TOOLS}      - installs the development tools used for the project'
>	@echo '  ${TEST}               - runs test suite for the project'
>	@echo '  ${ADD_LICENSE}        - adds license header to src files'
>	@echo '  ${DEB}                - generates the project binary debian package'
>	@echo '  ${CLEAN}              - remove files created by other targets'
>	@echo 'Common make configurations (e.g. make [config]=1 [targets]):'
>	@echo '  COPYRIGHT_HOLDERS     - string denoting copyright holder(s)/author(s)'
>	@echo '                          (e.g. "John Smith, Alice Smith" or "John Smith")'

${TARGET_EXEC}: debcomprt.go
>	${GO} build -o "${TARGET_EXEC}" -buildmode=pie -mod vendor

.PHONY: ${INSTALL}
${INSTALL}: ${TARGET_EXEC}
ifdef DPKG_INSTALL
>	${INSTALL} "${TARGET_EXEC}" "${DESTDIR}${bin_dir}"
else
>	${GO} install
endif

.PHONY: ${INSTALL_TOOLS}
${INSTALL_TOOLS}:
>	${GO} install -mod vendor ${GO_TOOLS}

.PHONY: ${TEST}
${TEST}:
>	sudo PATH="${PATH}" ${GO} test -v -mod vendor

.PHONY: ${ADD_LICENSE}
${ADD_LICENSE}:
>	@[ -n "${COPYRIGHT_HOLDERS}" ] || { echo "COPYRIGHT_HOLDERS was not passed into make"; exit 1; }
>	${ADDLICENSE} -l apache -c "${COPYRIGHT_HOLDERS}" ${src}

.PHONY: ${UPSTREAM_TARBALL}
${UPSTREAM_TARBALL}: ${_upstream_tarball_path}

${_upstream_tarball_path}:
>	mkdir --parents "${BUILD_DIR}"
>	tar zcf "$@" \
		--transform 's,^\.,${_upstream_tarball_prefix},' \
		--exclude=debian \
		--exclude=debian/* \
		--exclude="${BUILD_DIR}" \
		--exclude="${BUILD_DIR}"/* \
		--exclude-vcs-ignores \
		./.github ./.gitignore ./*

.PHONY: ${DEB}
${DEB}: ${_upstream_tarball_path}
>	cd "${BUILD_DIR}" \
>	&& mv "${_upstream_tarball}" "${_upstream_tarball_dash_to_underscore}" \
>	&& tar zxf "${_upstream_tarball_dash_to_underscore}" \
>	&& cd "${_upstream_tarball_prefix}" \
>	&& cp --recursive "${CURDIR}/debian" ./debian \
>	&& debuild -us -uc

.PHONY: ${CLEAN}
${CLEAN}:
>	rm --force "${TARGET_EXEC}"
>	rm --recursive --force "${BUILD_DIR}"
