cmake_minimum_required(VERSION 3.16)

file(READ "${CMAKE_SOURCE_DIR}/internal/app/version.json" VERSION_JSON_CONTENT)
string(REGEX MATCH "\"version\"[ \t]*:[ \t]*\"([^\"]*)\"" VERSION_MATCH "${VERSION_JSON_CONTENT}")
set(LUNEX_VERSION "${CMAKE_MATCH_1}")

project(lunex
    VERSION 0.0.0
    DESCRIPTION "Lunex lang"
    LANGUAGES NONE
)

set(GO_MIN_VERSION "1.25")
set(GO_INSTALL_VERSION "1.25.14")

if(CMAKE_HOST_SYSTEM_NAME STREQUAL "Linux")
    set(DETECTED_OS "linux")
    if(EXISTS "/system/bin/linker64" OR EXISTS "/system/bin/linker" OR
       EXISTS "/apex/com.android.runtime/bin/linker64" OR EXISTS "/apex/com.android.runtime/bin/linker" OR
       EXISTS "/system/bin/getprop")
        set(DETECTED_OS "android")
    elseif(DEFINED ENV{ANDROID_ROOT} AND DEFINED ENV{ANDROID_DATA})
        set(DETECTED_OS "android")
    elseif(DEFINED ENV{PREFIX})
        string(REGEX MATCH "^/data/(data/com\.termux|user/[^/]+/com\.termux|user_de/[^/]+/com\.termux)/" TERMUX_PREFIX_MATCH "$ENV{PREFIX}")
        if(TERMUX_PREFIX_MATCH)
            set(DETECTED_OS "android")
        endif()
    elseif(EXISTS "/proc/version")
        file(READ "/proc/version" PROC_VERSION_CONTENT)
        string(TOLOWER "${PROC_VERSION_CONTENT}" PROC_VERSION_LOWER)
        if(PROC_VERSION_LOWER MATCHES "android|bionic")
            set(DETECTED_OS "android")
        endif()
    endif()
elseif(CMAKE_HOST_SYSTEM_NAME STREQUAL "Darwin")
    set(DETECTED_OS "macos")
elseif(CMAKE_HOST_SYSTEM_NAME MATCHES "Windows")
    set(DETECTED_OS "windows")
elseif(CMAKE_HOST_SYSTEM_NAME STREQUAL "FreeBSD")
    set(DETECTED_OS "freebsd")
else()
    set(DETECTED_OS "unknown")
endif()

set(HOST_ARCH "${CMAKE_HOST_SYSTEM_PROCESSOR}")
if(HOST_ARCH STREQUAL "x86_64" OR HOST_ARCH STREQUAL "amd64" OR HOST_ARCH STREQUAL "AMD64")
    set(DETECTED_ARCH "amd64")
elseif(HOST_ARCH STREQUAL "aarch64" OR HOST_ARCH STREQUAL "arm64")
    set(DETECTED_ARCH "arm64")
elseif(HOST_ARCH STREQUAL "riscv64")
    set(DETECTED_ARCH "riscv64")
else()
    set(DETECTED_ARCH "unknown")
endif()

set(LUNEX_TARGET_OS "${DETECTED_OS}" CACHE STRING "Target OS for the Lunex build")
set(LUNEX_TARGET_ARCH "${DETECTED_ARCH}" CACHE STRING "Target architecture for the Lunex build")

if(LUNEX_TARGET_OS STREQUAL "macos")
    set(GOOS "darwin")
elseif(LUNEX_TARGET_OS STREQUAL "android")
    set(GOOS "android")
else()
    set(GOOS "${LUNEX_TARGET_OS}")
endif()

if(LUNEX_TARGET_ARCH STREQUAL "x86_64" OR LUNEX_TARGET_ARCH STREQUAL "amd64")
    set(GOARCH "amd64")
elseif(LUNEX_TARGET_ARCH STREQUAL "aarch64" OR LUNEX_TARGET_ARCH STREQUAL "arm64")
    set(GOARCH "arm64")
else()
    set(GOARCH "${LUNEX_TARGET_ARCH}")
endif()

if(LUNEX_TARGET_OS STREQUAL "windows")
    set(BIN_EXT ".exe")
else()
    set(BIN_EXT "")
endif()

set(GO_BUILD_MODE "")
if(LUNEX_TARGET_OS STREQUAL "android")
    set(GO_BUILD_MODE "-buildmode=pie")
endif()
set(GO_BUILD_FLAGS -trimpath -tags netgo,osusergo -ldflags=-s\ -w)
if(LUNEX_TARGET_OS STREQUAL "freebsd")
    list(APPEND GO_BUILD_FLAGS -gcflags=github.com/go-webgpu/goffi/internal/fakecgo=-std)
endif()

set(LUNEX_BIN "${CMAKE_SOURCE_DIR}/lunex${BIN_EXT}")
set(RELEASE_DIR "${CMAKE_SOURCE_DIR}/release")

find_program(GO_EXECUTABLE go)

if(GO_EXECUTABLE)
    execute_process(
        COMMAND ${GO_EXECUTABLE} version
        OUTPUT_VARIABLE GO_VERSION_OUTPUT
        OUTPUT_STRIP_TRAILING_WHITESPACE
    )
    string(REGEX MATCH "go([0-9]+\\.[0-9]+(\\.[0-9]+)?)" GO_VERSION_MATCH "${GO_VERSION_OUTPUT}")
    set(GO_INSTALLED_VERSION "${CMAKE_MATCH_1}")
    if(GO_INSTALLED_VERSION VERSION_LESS GO_MIN_VERSION)
        set(GO_NEEDS_INSTALL TRUE)
    else()
        set(GO_NEEDS_INSTALL FALSE)
        message(STATUS "Go ${GO_INSTALLED_VERSION} is already installed")
    endif()
else()
    set(GO_NEEDS_INSTALL TRUE)
endif()

if(GO_NEEDS_INSTALL)
    message(STATUS "Installing Go ${GO_MIN_VERSION}+")
    set(GO_ARCHIVE_NAME "go${GO_INSTALL_VERSION}.${GOOS}-${GOARCH}")
    set(GO_DOWNLOAD_URL "https://go.dev/dl/${GO_ARCHIVE_NAME}.tar.gz")
    set(GO_TMP_DIR "${CMAKE_BINARY_DIR}/go-download")
    file(MAKE_DIRECTORY "${GO_TMP_DIR}")
    file(DOWNLOAD "${GO_DOWNLOAD_URL}" "${GO_TMP_DIR}/go.tar.gz" SHOW_PROGRESS)
    execute_process(
        COMMAND ${CMAKE_COMMAND} -E tar xzf "${GO_TMP_DIR}/go.tar.gz"
        WORKING_DIRECTORY "${GO_TMP_DIR}"
    )
    file(COPY "${GO_TMP_DIR}/go/bin/go" DESTINATION "$ENV{HOME}/.local/bin")
    set(GO_EXECUTABLE "$ENV{HOME}/.local/bin/go")
    message(STATUS "Go ${GO_INSTALL_VERSION} installed")
endif()

add_custom_target(banner
    COMMAND ${CMAKE_COMMAND} -E echo ""
    COMMAND ${CMAKE_COMMAND} -E echo "  Lunex lang  v${LUNEX_VERSION}"
    COMMAND ${CMAKE_COMMAND} -E echo "  Created by David Dev"
    COMMAND ${CMAKE_COMMAND} -E echo "  GitHub: https://github.com/Megamexlevi2"
    COMMAND ${CMAKE_COMMAND} -E echo ""
)

add_custom_target(build ALL
    COMMAND ${CMAKE_COMMAND} -E env
        GONOSUMDB=* GOFLAGS=-mod=mod GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=0
        ${GO_EXECUTABLE} build ${GO_BUILD_MODE} ${GO_BUILD_FLAGS} -o "${LUNEX_BIN}" ./cmd/lunex
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    DEPENDS banner
    COMMENT "Building Lunex for ${GOOS}/${GOARCH}"
    VERBATIM
)

add_custom_target(release
    COMMAND ${CMAKE_COMMAND} -E make_directory "${RELEASE_DIR}"
    COMMAND ${CMAKE_COMMAND} -DGO_EXECUTABLE=${GO_EXECUTABLE} -DSRC_DIR=${CMAKE_SOURCE_DIR}
        -DOUT_DIR=${RELEASE_DIR} -P "${CMAKE_SOURCE_DIR}/cmake/BuildRelease.cmake"
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    DEPENDS banner
    COMMENT "Starting multi-platform release build"
    VERBATIM
)

add_custom_target(run_tests
    COMMAND ${CMAKE_COMMAND} -E env bash "${CMAKE_SOURCE_DIR}/tests/run_tests.sh"
    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
    DEPENDS build
    COMMENT "Running Lunex integration tests"
    VERBATIM
)

add_custom_target(clean_artifacts
    COMMAND ${CMAKE_COMMAND} -E rm -f "${CMAKE_SOURCE_DIR}/lunex" "${CMAKE_SOURCE_DIR}/lunex.exe"
    COMMAND ${CMAKE_COMMAND} -E rm -rf "${RELEASE_DIR}"
    COMMENT "Cleaning build artifacts"
    VERBATIM
)
