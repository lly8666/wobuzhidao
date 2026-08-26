cmake_minimum_required(VERSION 3.20)
project(wbd_dtls_shim C)

if(NOT DEFINED WOLFSSL_SOURCE_DIR)
  message(FATAL_ERROR "WOLFSSL_SOURCE_DIR is required")
endif()

# Match the frozen Linux configure contract: static wolfSSL with TLS 1.3 and
# DTLS 1.3 enabled. Keep these cache values explicit so Windows qualification
# cannot silently inherit a developer-machine wolfSSL configuration.
set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
set(WOLFSSL_TLS13 yes CACHE STRING "" FORCE)
set(WOLFSSL_DTLS yes CACHE STRING "" FORCE)
set(WOLFSSL_DTLS13 yes CACHE STRING "" FORCE)
set(WOLFSSL_EXAMPLES no CACHE STRING "" FORCE)
set(WOLFSSL_CRYPT_TESTS no CACHE STRING "" FORCE)

add_subdirectory("${WOLFSSL_SOURCE_DIR}" "${CMAKE_BINARY_DIR}/wolfssl-build" EXCLUDE_FROM_ALL)

add_executable(wbd_dtls_shim wbd_dtls_shim.c)
target_link_libraries(wbd_dtls_shim PRIVATE wolfssl)

if(WIN32)
  target_compile_definitions(wbd_dtls_shim PRIVATE _CRT_SECURE_NO_WARNINGS)
  target_link_libraries(wbd_dtls_shim PRIVATE ws2_32)
  if(MSVC)
    target_compile_options(wbd_dtls_shim PRIVATE /W4 /WX)
  endif()
else()
  target_link_libraries(wbd_dtls_shim PRIVATE m)
  target_compile_options(wbd_dtls_shim PRIVATE -Wall -Wextra -Werror)
endif()
