#include <hiredis/hiredis.h>
#include <stdio.h>
#include <unistd.h>

redisContext *get_connection() {
    // redisContext *c = redisConnect("172.17.0.3", 9900);
    redisContext *c = redisConnect("127.0.0.1", 9900);
    if (c != NULL && c->err) {
        printf("Error: %s\n", c->errstr);
        redisFree(c);
        c = NULL;
    } else {
        printf("Connected to Redis\n");
    }
    return c;    
}

int main () {
    int retries = 15;
    while (retries--) {
        redisContext *c = get_connection();
        if (c == NULL)
            goto next;
        char *namespaces[] = { "zdbfs-data", "zdbfs-meta", "zdbfs-temp" };
        for (int i = 0; i < sizeof(namespaces) / sizeof(char *); i++) {
            redisReply *reply;
            reply = redisCommand(c, "NSNEW %s", namespaces[i]);
            if (reply == NULL) {
                printf("Error adding %s: %s\n", namespaces[i], c->errstr);
                redisFree(c);
                c = get_connection();
                if (c == NULL)
                    goto next;
            }
            freeReplyObject(reply);
        }

        redisFree(c);
        goto done;
next:
        sleep(1);
        continue;
done:
        return 0;
    }
    return 1;
}
