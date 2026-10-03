set(RELEASE_TARGETS
    "linux;amd64"
    "linux;arm64"
    "windows;amd64"
    "darwin;amd64"
    "darwin;arm64"
    "android;arm64"
    "freebsd;amd64"
)

foreach(TARGET_PAIR ${RELEASE_TARGETS})
    string(REPLACE ";" "|" TARGET_PAIR_SAFE "${TARGET_PAIR}")
endforeach()

set(TARGET_LIST
    "linux:amd64"
    "linux:arm64"
    "windows:amd64"
    "windows:arm64"
    "darwin:amd64"
    "darwin:arm64"
    "android:arm64"
    "freebsd:amd64"
)

foreach(ENTRY ${TARGET_LIST})
    string(REPLACE ":" ";" ENTRY_LIST "${ENTRY}")
    list(GET ENTRY_LIST 0 T_OS)
    list(GET ENTRY_LIST 1 T_ARCH)

    set(T_EXT "")
    if(T_OS STREQUAL "windows")
        set(T_EXT ".exe")
    endif()

    set(RELEASE_NAME "lunex-${T_OS}-${T_ARCH}${T_EXT}")
    set(ANDROID_BUILD_MODE "")
    if(T_OS STREQUAL "android")
        set(ANDROID_BUILD_MODE "-buildmode=pie")
    endif()
    set(GO_BUILD_FLAGS -trimpath -tags netgo,osusergo -ldflags=-s\ -w)
    if(T_OS STREQUAL "freebsd")
        list(APPEND GO_BUILD_FLAGS -gcflags=github.com/go-webgpu/goffi/internal/fakecgo=-std)
    endif()
    message(STATUS "Building ${T_OS}/${T_ARCH}")

    execute_process(
        COMMAND ${CMAKE_COMMAND} -E env
            GONOSUMDB=* GOFLAGS=-mod=mod GOOS=${T_OS} GOARCH=${T_ARCH} CGO_ENABLED=0
            ${GO_EXECUTABLE} build ${ANDROID_BUILD_MODE} ${GO_BUILD_FLAGS}
            -o "${OUT_DIR}/${RELEASE_NAME}" ./cmd/lunex
        WORKING_DIRECTORY ${SRC_DIR}
        RESULT_VARIABLE BUILD_RESULT
    )

    if(NOT BUILD_RESULT EQUAL 0)
        message(FATAL_ERROR "Build failed for ${T_OS}/${T_ARCH}")
    endif()

    message(STATUS "Done: ${OUT_DIR}/${RELEASE_NAME}")
endforeach()

message(STATUS "All targets built successfully in ${OUT_DIR}")
