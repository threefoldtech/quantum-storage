#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#include <string.h>
#include <hiredis/hiredis.h>

void diep(char *str) {
    fprintf(stderr, "[-] %s: %s\n", str, strerror(errno));
    exit(EXIT_FAILURE);
}

int main(int argc, char **argv) {
    redisContext *ctx;
    char *host = "127.0.0.1";
    int port = 9900;

    if(getenv("ZDBCTL_HOST"))
        host = getenv("ZDBCTL_HOST");

    if(getenv("ZDBCTL_PORT"))
        port = atoi(getenv("ZDBCTL_PORT"));

    printf("[+] zdb: connecting [%s, %d]\n", host, port);

    if(!(ctx = redisConnect(host, port)))
        diep("zdb: connect");

    if(ctx->err) {
        printf("[-] zdb: %s\n", ctx->errstr);
        exit(EXIT_FAILURE);
    }

    redisReply *reply;

    if(!(reply = redisCommandArgv(ctx, argc - 1, argv + 1, NULL))) {
        printf("[-] zdb: %s", ctx->errstr);
        return 1;
    }

    printf(">> %s\n", reply->str);

    return 0;
}
