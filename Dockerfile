FROM ubuntu:20.04
WORKDIR /tmp

RUN apt-get update && \
    apt-get install -y build-essential musl-tools libfuse3-dev git libhiredis-dev curl unzip

RUN curl https://sh.rustup.rs -sSf | sh -s -- -y
RUN . $HOME/.cargo/env && rustup target add x86_64-unknown-linux-musl

RUN curl -L https://github.com/vlang/v/releases/latest/download/v_linux.zip -o /tmp/v_linux.zip && \
    cd /tmp && \
    unzip v_linux.zip

RUN ln -s /usr/include/hiredis /usr/include/x86_64-linux-musl/ && \
    ln -s /usr/include/linux /usr/include/x86_64-linux-musl/ && \
    ln -s /usr/include/x86_64-linux-gnu/asm /usr/include/x86_64-linux-musl/ && \
    ln -s /usr/include/asm-generic /usr/include/x86_64-linux-musl/

RUN git clone https://github.com/threefoldtech/0-db-fs
RUN git clone https://github.com/threefoldtech/0-db
RUN git clone https://github.com/threefoldtech/0-stor_v2
RUN git clone https://github.com/crystaluniverse/vlang_tools

RUN cd /tmp/0-db-fs && \
    CC=musl-gcc LDFLAGS=-L/usr/lib/x86_64-linux-gnu make release

RUN cd /tmp/0-db/libzdb && \
    CC=musl-gcc make release && \
    cd /tmp/0-db/zdbd && \
    CC=musl-gcc make release
                    
RUN cd /tmp/0-stor_v2 && \
    . $HOME/.cargo/env && \
    cargo build --target x86_64-unknown-linux-musl --release

COPY nscreate.v /tmp/vlang_tools/vlang_tools/
RUN cd /tmp/vlang_tools/vlang_tools && \
    /tmp/v/v nscreate.v -cc musl-gcc

FROM alpine:latest  
RUN apk --no-cache add fuse3 hiredis
WORKDIR /

COPY --from=0 /tmp/0-db-fs/zdbfs /bin/
COPY --from=0 /tmp/0-db/zdbd/zdb /bin/
COPY --from=0 /tmp/0-stor_v2/target/x86_64-unknown-linux-musl/release/zstor_v2 /bin/zstor
COPY --from=0 /tmp/vlang_tools/vlang_tools/nscreate /bin/

COPY default-sample.toml /etc/zstor-default.toml

RUN mkdir /lib/zdb
COPY zdb-hook.sh /lib/zdb/zstor-hook.sh
RUN chmod +x /lib/zdb/zstor-hook.sh

COPY init.sh /
RUN chmod +x /init.sh

CMD ["/init.sh"]
