# special makefile variables
.DEFAULT_GOAL := help
.RECIPEPREFIX := >

# recursive variables
SHELL = /usr/bin/sh
BUILD_DIR_NAME = build
BUILD_DIR = ./${BUILD_DIR_NAME}
DEBIAN_DIR = debian
DEBIAN_DIR_PATH = ./${DEBIAN_DIR}
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
DOCKER = docker
ENVSUBST = envsubst
ADDLICENSE = addlicense
executables = \
	${SUDO}\
	${GIT}\
	${GO}

# tools, inspired by:
# https://stackoverflow.com/questions/56636580/replace-retool-with-tools-go-for-multi-developer-and-ci-environments-using-go-mo#answer-56640587
REQ_GO_TOOLS = \
	github.com/cavcrosby/genruntime-vars
DEV_GO_TOOLS = \
	github.com/google/addlicense

# docker related variables
DOCKER_REPO = cavcrosby/debcomprt
DOCKER_REPO_CONTEXT = latest
DOCKER_LATEST_VERSION_TAG = v1.1.1

# gnu install directory variables, for reference:
# https://golang.org/doc/tutorial/compile-install
prefix = /usr/local
exec_prefix = ${prefix}
bin_dir = ${exec_prefix}/bin

# targets
HELP = help
SETUP = setup
INSTALL = install
INSTALL_TOOLS = install-tools
TEST = test
ADD_LICENSE = add-license
MAINTAINER_SCRIPTS = maintainer-scripts
DOCKER_IMAGE = docker-image
UPSTREAM_TARBALL = upstream-tarball
DEB = deb
CLEAN = clean

# to be passed in at make runtime
COPYRIGHT_HOLDERS =
IMAGE_RELEASE_BUILD =

# simply expanded variables
# inspired from:
# https://devconnected.com/how-to-list-git-tags/#Find_Latest_Git_Tag_Available
ifeq (${DEBCOMPRT_VERSION},)
	override DEBCOMPRT_VERSION := $(shell ${GIT} describe --tags --abbrev=0 | sed 's/v//')
else
	override DEBCOMPRT_VERSION := $(shell echo ${DEBCOMPRT_VERSION} | sed 's/v//')
endif

ifdef IMAGE_RELEASE_BUILD
	DOCKER_BUILD_OPTS = \
		--tag \
		${DOCKER_REPO}:${DOCKER_REPO_CONTEXT} \
		--tag \
		${DOCKER_REPO}:${DOCKER_LATEST_VERSION_TAG}-bullseye
else
	DOCKER_REPO_CONTEXT = test
	DOCKER_BUILD_OPTS = \
		--tag \
		${DOCKER_REPO}:${DOCKER_REPO_CONTEXT}
endif

src := $(shell find . \( -type f \) \
	-and \( -name '*.go' \) \
	-and \( -not -iregex '.*/vendor.*' \) \
)
_upstream_tarball_prefix = ${TARGET_EXEC}-${DEBCOMPRT_VERSION}
_upstream_tarball = ${_upstream_tarball_prefix}${UPSTREAM_TARBALL_EXT}
_upstream_tarball_dash_to_underscore = $(shell echo "${_upstream_tarball}" | awk --field-separator='-' '{print $$1"_"$$2}')
_upstream_tarball_path = ${BUILD_DIR}/${_upstream_tarball}

SHELL_TEMPLATE_EXT := .shtpl
shell_template_wildcard := %${SHELL_TEMPLATE_EXT}
# DISCUSS(cavcrosby): there are other implicit rules where I manually conjoin the
# path for the expected file(s) that are generated from shell template(s). I
# will want to look into making things more consistent. This includes perhaps
# putting the debian directory path into a variable.
maintainer_script_shell_templates := $(shell find ${DEBIAN_DIR_PATH} -name *${SHELL_TEMPLATE_EXT})

# Determines the maintainer script name(s) to be generated from the template(s).
# Short hand notation for string substitution: $(text:pattern=replacement).
_maintainer_scripts := $(maintainer_script_shell_templates:${SHELL_TEMPLATE_EXT}=)

# inspired from:
# https://stackoverflow.com/questions/5618615/check-if-a-program-exists-from-a-makefile#answer-25668869
_check_executables := $(foreach exec,${executables},$(if $(shell command -v ${exec}),pass,$(error "No ${exec} in PATH")))

.PHONY: ${HELP}
${HELP}:
	# inspired by the makefiles of the Linux kernel and Mercurial
>	@echo 'Common make targets:'
>	@echo '  ${SETUP}              - installs the dependencies for this project'
>	@echo '  ${TARGET_EXEC}          - the ${TARGET_EXEC} binary'
>	@echo '  ${INSTALL}            - installs the decomprt binary and other needed files'
>	@echo '  ${UNINSTALL}          - uninstalls the decomprt binary and other needed files'
>	@echo '  ${INSTALL_TOOLS}      - installs optional development tools used for the project'
>	@echo '  ${TEST}               - runs test suite for the project'
>	@echo '  ${ADD_LICENSE}        - adds license header to src files'
>	@echo '  ${DOCKER_IMAGE}       - creates the docker image used to make the project'\''s'
>	@echo '                       debian packages '
>	@echo '  ${DEB}                - generates the project'\''s debian package(s)'
>	@echo '  ${CLEAN}              - remove files created by other targets'
>	@echo 'Common make configurations (e.g. make [config]=1 [targets]):'
>	@echo '  COPYRIGHT_HOLDERS     - string denoting copyright holder(s)/author(s)'
>	@echo '                          (e.g. "John Smith, Alice Smith" or "John Smith")'
>	@echo '  IMAGE_RELEASE_BUILD   - if set, this will cause targets dealing with docker'
>	@echo '                          images to work with the :latest image'

.PHONY: ${SETUP}
${SETUP}:
>	${GO} install -mod vendor ${REQ_GO_TOOLS}

${TARGET_EXEC}: debcomprt.go
>	${GO} generate -mod=vendor
>	${GO} build -o "${target_exec_path}" -buildmode=pie -mod vendor

.PHONY: ${INSTALL}
${INSTALL}: ${TARGET_EXEC}
>	${SUDO} ${INSTALL} "${target_exec_path}" "${DESTDIR}${bin_dir}"

.PHONY: ${UNINSTALL}
${UNINSTALL}:
>	${SUDO} rm --force "${DESTDIR}${bin_dir}/${TARGET_EXEC}"

.PHONY: ${INSTALL_TOOLS}
${INSTALL_TOOLS}:
>	${GO} install -mod vendor ${DEV_GO_TOOLS}

.PHONY: ${TEST}
${TEST}:
	# Trying to expand PATH once in root's shell does not seem to work. Hence the
	# command substitution to get root's PATH.
	#
	# bin_dir may already be in root's PATH, but that's ok.
>	${GO} generate -mod=vendor
>	${SUDO} --shell PATH="${bin_dir}:$$(sudo --shell echo \$$PATH)" ${GO} test -v -mod vendor

.PHONY: ${ADD_LICENSE}
${ADD_LICENSE}:
>	@[ -n "${COPYRIGHT_HOLDERS}" ] || { echo "COPYRIGHT_HOLDERS was not passed into make"; exit 1; }
>	${ADDLICENSE} -l apache -c "${COPYRIGHT_HOLDERS}" ${src}

# TODO(cavcrosby): variables are case sensitive in makefiles. That said, I
# believe relying on the case sensitivity to be asking for trouble. This should
# be refactored in other projects as well to use a underscore as prefix to the
# variable name.
.PHONY: ${MAINTAINER_SCRIPTS}
${MAINTAINER_SCRIPTS}: ${_maintainer_scripts}

${_maintainer_scripts}: ${maintainer_script_shell_templates}
>	${ENVSUBST} '${maintainer_scripts_vars}' < "$<" > "$@"

.PHONY: ${DOCKER_IMAGE}
${DOCKER_IMAGE}:
>	${DOCKER} build \
		--build-arg BRANCH="$$(git branch --show-current)" \
		--build-arg COMMIT="$$(git show --format=%h --no-patch)" \
		${DOCKER_BUILD_OPTS} \
		.

.PHONY: ${UPSTREAM_TARBALL}
${UPSTREAM_TARBALL}: ${_upstream_tarball_path}

${_upstream_tarball_path}: ${MAINTAINER_SCRIPTS}
>	mkdir --parents "${BUILD_DIR}"
>	tar zcf "$@" \
		--transform 's,^\.,${_upstream_tarball_prefix},' \
		--exclude=${DEBIAN_DIR_PATH} \
		--exclude=${DEBIAN_DIR_PATH}/* \
		--exclude="${BUILD_DIR}" \
		--exclude="${BUILD_DIR}"/* \
		--exclude-vcs-ignores \
		./.github ./.gitignore ./*

.PHONY: ${DEB}
${DEB}: ${_upstream_tarball_path}
	# DISCUSS(cavcrosby): add the debian source package to be uploaded as well to
	# artifactory.
	# DISCUSS(cavcrosby): inspect other projects where shell templates are used. In
	# the case that shell templates are used for normal shell scripts, replace each
	# double quoted shell variable to be evaluated by envsubst in single quotes to
	# differentiate those to be evaluated by envsubst and those not.
>	cd "${BUILD_DIR}" \
>	&& mv "${_upstream_tarball}" "${_upstream_tarball_dash_to_underscore}" \
>	&& tar zxf "${_upstream_tarball_dash_to_underscore}" \
>	&& cp --recursive "${CURDIR}/${DEBIAN_DIR}" "${_upstream_tarball_prefix}/${DEBIAN_DIR}" \
>	&& ${DOCKER} run \
		--volume "${CURDIR}/${BUILD_DIR_NAME}:/debcomprt/build" \
		--env EXTRACTED_UPSTREAM_TARBALL="${_upstream_tarball_prefix}" \
		--env LOCAL_USER_ID="$$(id --user)" \
		--env LOCAL_GROUP_ID="$$(id --group)" \
		--env DEBCOMPRT_VERSION="${DEBCOMPRT_VERSION}" \
		--rm --name debcomprt \
		${DOCKER_REPO}:${DOCKER_REPO_CONTEXT}

.PHONY: ${CLEAN}
${CLEAN}:
>	${SUDO} rm --recursive --force "${BUILD_DIR}"
	# match filename(s) generated from their respective shell template(s)
>	rm --force $$(find ./debian | grep --only-matching --perl-regexp '[\w\.\/]+(?=\.shtpl)')
>	rm --force "${RUNTIME_VARS_FILE}"
