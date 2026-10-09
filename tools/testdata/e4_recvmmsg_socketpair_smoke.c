// Functional fixture ONLY: two recvmmsg calls on a local AF_UNIX socketpair.
// No network interface, product binary, actual payload or throughput benchmark.
#define _GNU_SOURCE
#include <errno.h>
#include <stdio.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/uio.h>
#include <time.h>
#include <unistd.h>

int main(void) {
  int fd[2];
  if (socketpair(AF_UNIX, SOCK_DGRAM, 0, fd) != 0) return 11;
  // Parent starts an eBPF tracepoint program BEFORE releasing stdin.
  if (getchar() != 'G') return 12;
  char value = 'x';
  char recvbuf[8] = {0};
  struct iovec iov = {.iov_base=recvbuf, .iov_len=sizeof(recvbuf)};
  struct mmsghdr msg = {0};
  msg.msg_hdr.msg_iov = &iov;
  msg.msg_hdr.msg_iovlen = 1;
  if (send(fd[1], &value, 1, 0) != 1) return 13;
  if (recvmmsg(fd[0], &msg, 1, MSG_DONTWAIT, NULL) != 1) return 14;
  // Target one >20ms gap outside the kernel, not a slow blocking recv.
  struct timespec nap = {.tv_sec = 0, .tv_nsec=60000000};
  nanosleep(&nap, NULL);
  msg.msg_len = 0;
  if (send(fd[1], &value, 1, 0) != 1) return 15;
  if (recvmmsg(fd[0], &msg, 1, MSG_DONTWAIT, NULL) != 1) return 16;
  close(fd[0]);
  close(fd[1]);
  return 0;
}
