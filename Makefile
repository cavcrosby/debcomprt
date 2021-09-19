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
bin_dir = ${exec_prefix}

# targets
HELP = help
SETUP = setup
INSTALL = install
INSTALL_TOOLS = install-tools
TEST = test
ADD_LICENSE = add-license
UPSTREAM_TARBALL = upstream-tarball
DEBSOURCE = debsource
DEB = deb
CLEAN = clean

# to be passed in at make runtime
COPYRIGHT_HOLDERS =

# simply expanded variables
# inspired from:
# https://devconnected.com/how-to-list-git-tags/#Find_Latest_Git_Tag_Available
# version = $(shell ${GIT} describe --tags --abbrev=0 | sed 's/v//')
# TODO(cavcrosby): temp until tagging can be sorted.
version = 1.0
_upstream_tarball_prefix = ${TARGET_EXEC}_${version}
_upstream_tarball_path = ${BUILD_DIR}/${_upstream_tarball_prefix}${UPSTREAM_TARBALL_EXT}
src := $(shell find . \( -type f \) -and \( -iname '*.go' \) -and \( -not -iregex '.*/vendor.*' \))

# inspired from:
# https://stackoverflow.com/questions/5618615/check-if-a-program-exists-from-a-makefile#answer-25668869
_check_executables := $(foreach exec,${executables},$(if $(shell command -v ${exec}),pass,$(error "No ${exec} in PATH")))

.PHONY: ${HELP}
${HELP}:
	# inspired by the makefiles of the Linux kernel and Mercurial
>	@echo 'Available make targets:'
>	@echo '  ${TARGET_EXEC}          - the ${TARGET_EXEC} binary'
>	@echo '  ${INSTALL}            - installs the local decomprt binary (pathing: ${prefix})'
>	@echo '  ${INSTALL_TOOLS}      - installs the development tools used for the project'
>	@echo '  ${TEST}               - runs test suite for the project'
>	@echo '  ${ADD_LICENSE}        - adds license header to src files'
>	@echo '  ${CLEAN}              - remove files created by other targets'
>	@echo 'Public make configurations (e.g. make [config]=1 [targets]):'
>	@echo '  COPYRIGHT_HOLDERS     - string denoting copyright holder(s)/author(s)'
>	@echo '                          (e.g. "John Smith, Alice Smith" or "John Smith")'

${TARGET_EXEC}: debcomprt.go
>	${GO} build -o "${TARGET_EXEC}" -mod vendor

.PHONY: ${INSTALL}
${INSTALL}: ${TARGET_EXEC}
>	${GO} install

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
>	tar czvf "$@" \
		--exclude=debian \
		--exclude=debian/* \
		--exclude-vcs-ignores \
		.github .gitignore *

.PHONY: ${DEBSOURCE}
${DEBSOURCE}: ${_upstream_tarball_path}
>	@cd "${BUILD_DIR}" \
>	&& mkdir --parents "${_upstream_tarball_prefix}" \
>	&& tar zxvf "$$(basename ${_upstream_tarball_path})" -C "${_upstream_tarball_prefix}" \
>	&& cd "${_upstream_tarball_prefix}" \
>	&& debuild -us -uc

.PHONY: ${CLEAN}
${CLEAN}:
>	rm --force "${TARGET_EXEC}"
>	rm --recursive --force "${BUILD_DIR}"
