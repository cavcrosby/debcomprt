# special makefile variables
.DEFAULT_GOAL := help
.RECIPEPREFIX := >

# recursive variables
SHELL = /usr/bin/sh
BUILD_DIR = ./build
CONFIGS_DIR = ./configs
TARGET_EXEC = debcomprt
target_exec_path = ${BUILD_DIR}/${TARGET_EXEC}
export PROG_DATA_DIR = /usr/local/share/debcomprt
UPSTREAM_TARBALL_EXT = .orig.tar.gz

# DISCUSS(cavcrosby): according to the make manual, every makefile should define
# the INSTALL variable. Perhaps look into this later? How does this affect
# previous makefiles?
# executables
GO = go
GIT = git
SUDO = sudo
ADDLICENSE = addlicense
ENVSUBST = envsubst
executables = \
	${GIT}\
	${GO}

# tools, inspired by:
# https://stackoverflow.com/questions/56636580/replace-retool-with-tools-go-for-multi-developer-and-ci-environments-using-go-mo#answer-56640587
GO_TOOLS = github.com/google/addlicense

# gnu install directory variables, for reference:
# https://golang.org/doc/tutorial/compile-install
prefix = /usr/local
sysconfdir = ${prefix}/etc
exec_prefix = ${prefix}
bin_dir = ${exec_prefix}/bin

# 1. installing any config files should be more explicity done in this make file
# 2. somehow link the program data dir in with the binary (e.g. jailtime has a good example of doing this, allows us in not having to make go source into templates)

# targets
HELP = help
INSTALL = install
UNINSTALL = uninstall
INSTALL_TOOLS = install-tools
TEST = test
CONFIGS = configs
ADD_LICENSE = add-license
UPSTREAM_TARBALL = upstream-tarball
DEB = deb
CLEAN = clean

# to be passed in at make runtime
COPYRIGHT_HOLDERS =

# simply expanded variables
# inspired from:
# https://devconnected.com/how-to-list-git-tags/#Find_Latest_Git_Tag_Available
ifeq (${version},)
	override version := $(shell ${GIT} describe --tags --abbrev=0 | sed 's/v//')
else
	override version := $(shell echo ${version} | sed 's/v//')
endif

src := $(shell find . \( -type f \) -and \( -iname '*.go' \) -and \( -not -iregex '.*/vendor.*' \))
_upstream_tarball_prefix = ${TARGET_EXEC}-${version}
_upstream_tarball = ${_upstream_tarball_prefix}${UPSTREAM_TARBALL_EXT}
_upstream_tarball_dash_to_underscore = $(shell echo "${_upstream_tarball}" | awk --field-separator='-' '{print $$1"_"$$2}')
_upstream_tarball_path = ${BUILD_DIR}/${_upstream_tarball}

JSON_EXT := .json
SHELL_TEMPLATE_EXT := .shtpl
json_shell_template_ext := ${JSON_EXT}${SHELL_TEMPLATE_EXT}

# inspired from:
# https://stackoverflow.com/questions/5618615/check-if-a-program-exists-from-a-makefile#answer-25668869
_check_executables := $(foreach exec,${executables},$(if $(shell command -v ${exec}),pass,$(error "No ${exec} in PATH")))

.PHONY: ${HELP}
${HELP}:
	# inspired by the makefiles of the Linux kernel and Mercurial
>	@echo 'Common make targets:'
>	@echo '  ${TARGET_EXEC}          - the ${TARGET_EXEC} binary'
>	@echo '  ${INSTALL}            - installs the decomprt binary and other needed files'
>	@echo '  ${UNINSTALL}          - uninstalls the decomprt binary and other needed files'
>	@echo '  ${INSTALL_TOOLS}      - installs the development tools used for the project'
>	@echo '  ${TEST}               - runs test suite for the project'
>	@echo '  ${ADD_LICENSE}        - adds license header to src files'
>	@echo '  ${DEB}                - generates the project'\''s debian package(s)'
>	@echo '  ${CLEAN}              - remove files created by other targets'
>	@echo 'Common make configurations (e.g. make [config]=1 [targets]):'
>	@echo '  COPYRIGHT_HOLDERS     - string denoting copyright holder(s)/author(s)'
>	@echo '                          (e.g. "John Smith, Alice Smith" or "John Smith")'

${TARGET_EXEC}: debcomprt.go
>	${GO} generate -mod vendor ./...
>	${GO} build -o "${target_exec_path}" -buildmode=pie -mod vendor

.PHONY: ${INSTALL}
${INSTALL}: ${TARGET_EXEC} ${CONFIGS}
>	${SUDO} ${INSTALL} "${target_exec_path}" "${DESTDIR}${bin_dir}"
>	${SUDO} ${INSTALL} --mode=644 "${CONFIGS_DIR}/debcomprt${JSON_EXT}" "${DESTDIR}${sysconfdir}/debcomprt"

.PHONY: ${UNINSTALL}
${UNINSTALL}:
>	${SUDO} rm --force "${DESTDIR}${bin_dir}/${TARGET_EXEC}"
>	${SUDO} rm --recursive --force "${DESTDIR}${sysconfdir}/debcomprt"

.PHONY: ${INSTALL_TOOLS}
${INSTALL_TOOLS}:
>	${GO} install -mod vendor ${GO_TOOLS}

.PHONY: ${TEST}
${TEST}:
	# Trying to expand PATH once in root's shell does not seem to work. Hence the
	# command substitution to get root's PATH.
	#
	# bin_dir may already be in root's PATH, but that's ok.
>	${GO} generate -mod vendor ./...
>	${SUDO} --shell PATH="${bin_dir}:$$(sudo --shell echo \$$PATH)" ${GO} test -v -mod vendor

.PHONY: ${ADD_LICENSE}
${ADD_LICENSE}:
>	@[ -n "${COPYRIGHT_HOLDERS}" ] || { echo "COPYRIGHT_HOLDERS was not passed into make"; exit 1; }
>	${ADDLICENSE} -l apache -c "${COPYRIGHT_HOLDERS}" ${src}

.PHONY: ${CONFIGS}
${CONFIGS}:
>	${ENVSUBST} '$${PROG_DATA_DIR}' < "${CONFIGS_DIR}/debcomprt${json_shell_template_ext}" > "${CONFIGS_DIR}/debcomprt${JSON_EXT}"

.PHONY: ${UPSTREAM_TARBALL}
${UPSTREAM_TARBALL}: ${_upstream_tarball_path}

${_upstream_tarball_path}: ${CONFIGS}
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
	# TODO(cavcrosby): lintian now issues the following error when building a deb package:
	# 'debcomprt changes: bad-distribution-in-changes-file stable'.
	# This should be resolved, though I am partially curious if this error exists for
	# 'v2.0.0' of the package.
>	cd "${BUILD_DIR}" \
>	&& mv "${_upstream_tarball}" "${_upstream_tarball_dash_to_underscore}" \
>	&& tar zxf "${_upstream_tarball_dash_to_underscore}" \
>	&& cd "${_upstream_tarball_prefix}" \
>	&& cp --recursive "${CURDIR}/debian" ./debian \
>	&& debuild --rootcmd=sudo --unsigned-source --unsigned-changes

.PHONY: ${CLEAN}
${CLEAN}:
>	sudo rm --recursive --force "${BUILD_DIR}"
>	rm --force "${CONFIGS_DIR}/debcomprt${JSON_EXT}"
