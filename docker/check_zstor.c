#include <stdio.h>
#include <unistd.h>
#include <sys/socket.h>
#include <string.h>
#include <sys/types.h>
#include <sys/un.h>

const char *zstor_socket = "/var/run/zstor.sock";

int main() {
	int fd;
    struct sockaddr_un addr;
    if ((fd = socket(PF_UNIX, SOCK_STREAM, 0)) < 0) {
        perror("socket");
        return 1;
    }
    memset(&addr, 0, sizeof(addr));
	addr.sun_family = AF_UNIX;
	strcpy(addr.sun_path, zstor_socket);
	if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) == -1) {
		perror("connect");
		return 1;
	}
    close(fd); // ignore errors
	return 0;
}
