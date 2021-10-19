# special makefile variables
.DEFAULT_GOAL := help
.RECIPEPREFIX := >

# recursive variables
SHELL = /usr/bin/sh
BUILD_DIR = ./build
TARGET_EXEC = debcomprt
target_exec_path = ${BUILD_DIR}/${TARGET_EXEC}
export PROG_DATA_DIR = /usr/local/share/debcomprt
export RUNTIME_VARS_FILE = runtime_vars.go
UPSTREAM_TARBALL_EXT = .orig.tar.gz

# common vars to be used in packaging maintainer scripts
_PROG_DATA_DIR = $${PROG_DATA_DIR}
maintainer_scripts_vars = \
	${_PROG_DATA_DIR}

# DISCUSS(cavcrosby): according to the make manual, every makefile should define
# the INSTALL variable. Perhaps look into this later? How does this affect
# previous makefiles?
# executables
GO = go
GIT = git
SUDO = sudo
ENVSUBST = envsubst
ADDLICENSE = addlicense
executables = \
	${GIT}\
	${GO}

# tools, inspired by:
# https://stackoverflow.com/questions/56636580/replace-retool-with-tools-go-for-multi-developer-and-ci-environments-using-go-mo#answer-56640587
GO_TOOLS = github.com/cavcrosby/genruntime-vars
OPT_GO_TOOLS = github.com/google/addlicense

# gnu install directory variables, for reference:
# https://golang.org/doc/tutorial/compile-install
prefix = /usr/local
exec_prefix = ${prefix}
bin_dir = ${exec_prefix}/bin

# targets
HELP = help
INSTALL = install
UNINSTALL = uninstall
INSTALL_TOOLS = install-tools
INSTALL_NEEDED_TOOLS = install-needed-tools
TEST = test
ADD_LICENSE = add-license
MAINTAINER_SCRIPTS = maintainer-scripts
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

SHELL_TEMPLATE_EXT := .shtpl
shell_template_wildcard := %${SHELL_TEMPLATE_EXT}
# DISCUSS(cavcrosby): there are other cases where I manually conjoin the path for
# the expected file(s) to be generated from shell template(s). I will want to look
# into making things more consistent. This includes perhaps putting the debian
# directory path into a variable.
maintainer_script_shell_templates := $(shell find ./debian -name *${SHELL_TEMPLATE_EXT})

# Determines the maintainer script name(s) to be generated from the template(s).
# Short hand notation for string substitution: $(text:pattern=replacement).
maintainer_scripts := $(maintainer_script_shell_templates:${SHELL_TEMPLATE_EXT}=)

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
>	@echo '  ${INSTALL_TOOLS}      - installs optional development tools used for the project'
>	@echo '  ${TEST}               - runs test suite for the project'
>	@echo '  ${ADD_LICENSE}        - adds license header to src files'
>	@echo '  ${DEB}                - generates the project'\''s debian package(s)'
>	@echo '  ${CLEAN}              - remove files created by other targets'
>	@echo 'Common make configurations (e.g. make [config]=1 [targets]):'
>	@echo '  COPYRIGHT_HOLDERS     - string denoting copyright holder(s)/author(s)'
>	@echo '                          (e.g. "John Smith, Alice Smith" or "John Smith")'

.PHONY: ${INSTALL_NEEDED_TOOLS}
${INSTALL_NEEDED_TOOLS}:
>	${GO} install -mod vendor ${GO_TOOLS}

${TARGET_EXEC}: debcomprt.go ${INSTALL_NEEDED_TOOLS}
>	go generate -mod=vendor
>	${GO} build -o "${target_exec_path}" -buildmode=pie -mod vendor

.PHONY: ${INSTALL}
${INSTALL}: ${TARGET_EXEC}
>	${SUDO} ${INSTALL} "${target_exec_path}" "${DESTDIR}${bin_dir}"

.PHONY: ${UNINSTALL}
${UNINSTALL}:
>	${SUDO} rm --force "${DESTDIR}${bin_dir}/${TARGET_EXEC}"

.PHONY: ${INSTALL_TOOLS}
${INSTALL_TOOLS}:
>	${GO} install -mod vendor ${OPT_GO_TOOLS}

.PHONY: ${TEST}
${TEST}: ${INSTALL_NEEDED_TOOLS}
	# Trying to expand PATH once in root's shell does not seem to work. Hence the
	# command substitution to get root's PATH.
	#
	# bin_dir may already be in root's PATH, but that's ok.
>	go generate -mod=vendor
>	${SUDO} --shell PATH="${bin_dir}:$$(sudo --shell echo \$$PATH)" ${GO} test -v -mod vendor

.PHONY: ${ADD_LICENSE}
${ADD_LICENSE}: ${INSTALL_TOOLS}
>	@[ -n "${COPYRIGHT_HOLDERS}" ] || { echo "COPYRIGHT_HOLDERS was not passed into make"; exit 1; }
>	${ADDLICENSE} -l apache -c "${COPYRIGHT_HOLDERS}" ${src}

.PHONY: ${MAINTAINER_SCRIPTS}
${MAINTAINER_SCRIPTS}: ${maintainer_scripts}

${maintainer_scripts}: ${maintainer_script_shell_templates}
>	${ENVSUBST} '${maintainer_scripts_vars}' < "$<" > "$@"

.PHONY: ${UPSTREAM_TARBALL}
${UPSTREAM_TARBALL}: ${_upstream_tarball_path}

${_upstream_tarball_path}: ${MAINTAINER_SCRIPTS}
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
	# DISCUSS(cavcrosby): add the debian source package to be uploaded as well to
	# artifactory.
	# DISCUSS(cavcrosby): inspect other projects where shell templates are used. In
	# the case that shell templates are used for normal shell scripts, replace each
	# double quoted shell variable to be evaluated by envsubst in single quotes to
	# differentiate those to be evaluated by envsubst and those not.
>	cd "${BUILD_DIR}" \
>	&& mv "${_upstream_tarball}" "${_upstream_tarball_dash_to_underscore}" \
>	&& tar zxf "${_upstream_tarball_dash_to_underscore}" \
>	&& cd "${_upstream_tarball_prefix}" \
>	&& cp --recursive "${CURDIR}/debian" ./debian \
>	&& debuild --rootcmd=sudo --unsigned-source --unsigned-changes

.PHONY: ${CLEAN}
${CLEAN}:
>	${SUDO} rm --recursive --force "${BUILD_DIR}"
	# match filename(s) generated from their respective shell template(s)
>	rm --force $$(find ./debian | grep --only-matching --perl-regexp '[\w\.\/]+(?=\.shtpl)')
>	rm --force "${RUNTIME_VARS_FILE}"
