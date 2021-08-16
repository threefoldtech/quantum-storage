#include <stdio.h>
#include <stdlib.h>
#include <hiredis/hiredis.h>

void diep(char *str) {
    perror(str);
    exit(EXIT_FAILURE);
}

int main(int argc, char **argv) {
    redisContext *ctx;
    char *host = "127.0.0.1";
    int port = 9900;

    printf("[+] zdb: connecting [%s, %d]\n", host, port);

    if(!(ctx = redisConnect(host, port)))
        diep("zdb: connect: metadata");

    if(ctx->err) {
        printf("[-] zdb: %s", ctx->errstr);
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
