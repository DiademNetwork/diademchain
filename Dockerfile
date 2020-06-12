FROM frolvlad/alpine-glibc:alpine-3.7

ARG BUILD_NUMBER

EXPOSE 26656/tcp 6656/tcp 46657/tcp 46658/tcp

WORKDIR /app/

ADD https://private.delegatecall.com/diadem/linux/build-${BUILD_NUMBER}/diadem /usr/bin/

RUN mkdir /app/contracts \
    && chmod +x /usr/bin/diadem \
    && sync \
    && diadem init

CMD ["diadem", "run"]
