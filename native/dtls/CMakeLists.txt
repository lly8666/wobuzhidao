cmake_minimum_required(VERSION 3.20)
project(wbd_dtls_shim C)

if(NOT DEFINED WOLFSSL_SOURCE_DIR)
  message(FATAL_ERROR "WOLFSSL_SOURCE_DIR is required")
endif()
if(NOT DEFINED WOLFSSL_BUILD_DIR)
  message(FATAL_ERROR "WOLFSSL_BUILD_DIR is required")
endif()
if(NOT DEFINED WOLFSSL_LIBRARY)
  message(FATAL_ERROR "WOLFSSL_LIBRARY is required")
endif()

add_executable(wbd_dtls_shim wbd_dtls_shim.c)
target_include_directories(wbd_dtls_shim PRIVATE
  "${WOLFSSL_BUILD_DIR}"
  "${WOLFSSL_SOURCE_DIR}"
)
target_link_libraries(wbd_dtls_shim PRIVATE "${WOLFSSL_LIBRARY}")

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
