FROM golang:1.17-bullseye

ARG BRANCH
ARG COMMIT
LABEL tech.cavcrosby.debcomprt.branch="${BRANCH}"
LABEL tech.cavcrosby.debcomprt.commit="${COMMIT}"
LABEL tech.cavcrosby.debcomprt.vcs-repo="https://github.com/cavcrosby/debcomprt"

# create user that will build software from project
ENV BUILDER_USER_NAME="builder"
ENV builder_group_name="${BUILDER_USER_NAME}"
ENV builder_user_home="/home/${BUILDER_USER_NAME}"
ENV WORKING_DIR "/debcomprt/build"
WORKDIR "${WORKING_DIR}"

RUN apt-get update && apt-get install --assume-yes \
    build-essential \
    debhelper \
    devscripts \
    git \
    sudo

# In the event the base image does not contain a 'sh' binary in /usr/bin.
#
# Links the go tool to a directory that is in root's secure_path (sudoers file).
#
# MONITOR(cavcrosby): dpkg-shlibdeps associates a package version for libc6
# that does not currently exist in bulleye's standard repos. At this time, the
# binary control file inserts a libc6 version of 2.3.2/2.32 whereas bullseye
# repos only have up to 2.31/2.3.1. The dpkg (dynamic library to package
# versioning) 'symbols' file appears to be the first metadata used by
# dpkg-shlibdeps to determine the packaging version. I am unsure of the syntax
# used by this file, but a workaround was noticed when having dpkg-shlibdeps use
# the dpkg 'shlibs' file instead for libc. This appears only possible by removing
# libc's dpkg symbols file. This will need to be monitored in the event if the
# symbols file gets corrected over time.
RUN ln --symbolic --force /bin/sh /usr/bin/sh \
    && ln --symbolic --force /usr/local/go/bin/go /usr/bin/go \
    && rm /var/lib/dpkg/info/libc6:amd64.symbols

RUN echo "Defaults       env_keep += \"DEBCOMPRT_VERSION\"" > "/etc/sudoers.d/${BUILDER_USER_NAME}" \
    && echo "${BUILDER_USER_NAME} ALL=(ALL:ALL) PASSWD: ALL, NOPASSWD: /debcomprt/build/debcomprt-*/debian/rules" >> "/etc/sudoers.d/${BUILDER_USER_NAME}"

# If this entrypoint gets any bigger, then I am ok going to a entrypoint script.
# While it is possible to perhaps use su/sudo with wrapping commands in escaped
# quotes, that will make this pretty hard to read. Which I would like to avoid.
# For reference:
# https://stackoverflow.com/questions/52817150/dockerfile-entrypoint-unable-to-switch-user
ENTRYPOINT [ \
    "/bin/bash", \
    "-c", \
    "cd ${EXTRACTED_UPSTREAM_TARBALL} \
        && groupadd --gid \"${LOCAL_GROUP_ID}\" \"${builder_group_name}\" \
        && useradd --create-home --home-dir \"${builder_user_home}\" --uid \"${LOCAL_USER_ID}\" --gid \"${LOCAL_GROUP_ID}\" --shell /bin/bash \"${BUILDER_USER_NAME}\" \
        && sudo --shell --user \"${BUILDER_USER_NAME}\" make setup \
        && sudo --shell --user \"${BUILDER_USER_NAME}\" -- debuild --preserve-envvar='DEBCOMPRT_VERSION' --rootcmd=sudo --unsigned-source --unsigned-changes" \
]
