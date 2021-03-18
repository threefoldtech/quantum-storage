apt-get update
apt-get install -y wget unzip build-essential libssl-dev

VROOT=/opt
VPATH=$VROOT/v

cd $VROOT
wget https://github.com/vlang/v/releases/latest/download/v_linux.zip

export PATH=$VPATH:$PATH

v -o bootstrap.c bootstrap.v

gcc bootstrap.c $VPATH/thirdparty/cJSON/*.c -o bootstrap -I$VPATH/thirdparty/cJSON/ \
  -Wl,-Bstatic -lssl -lcrypto -Wl,-Bdynamic -ldl -lpthread
