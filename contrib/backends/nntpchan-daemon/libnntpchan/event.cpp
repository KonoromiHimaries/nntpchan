#include <fcntl.h>
#include <cstdlib>
#include <cassert>
#include <sys/types.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <netinet/in.h>

constexpr std::size_t ev_buffsz = 512;

#ifdef __linux__
#include "epoll.hpp"
typedef nntpchan::ev::EpollLoop<ev_buffsz> LoopImpl;
#else
#ifdef __FreeBSD__
#include "kqueue.hpp"
typedef nntpchan::ev::KqueueLoop<ev_buffsz> LoopImpl;
#else
#ifdef __netbsd__
typedef nntpchan::ev::KqueueLoop<ev_buffsz> LoopImpl;
#else
#error "unsupported platform"
#endif
#endif
#endif

namespace nntpchan
{
namespace ev
{
bool ev::Loop::BindTCP(const sockaddr *addr, ev::io *handler)
{
  assert(handler->acceptable());
  socklen_t slen;
  switch (addr->sa_family)
  {
  case AF_INET:
    slen = sizeof(sockaddr_in);
    break;
  case AF_INET6:
    slen = sizeof(sockaddr_in6);
    break;
  case AF_UNIX:
    slen = sizeof(sockaddr_un);
    break;
  default:
    return false;
  }
  int fd = socket(addr->sa_family, SOCK_STREAM | SOCK_NONBLOCK, 0);
  if (fd == -1)
  {
    return false;
  }

  if (bind(fd, addr, slen) == -1)
  {
    ::close(fd);
    return false;
  }

  if (listen(fd, 5) == -1)
  {
    ::close(fd);
    return false;
  }

  handler->fd = fd;
  return TrackConn(handler);
}

bool Loop::SetNonBlocking(ev::io *handler)
{
  return fcntl(handler->fd, F_SETFL, fcntl(handler->fd, F_GETFL, 0) | O_NONBLOCK) != -1;
}
}

ev::Loop *NewMainLoop() { return new LoopImpl; }
}