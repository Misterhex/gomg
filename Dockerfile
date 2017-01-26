FROM golang

RUN apt-get update -y
RUN apt-get upgrade -y
RUN apt-get install libjpeg-progs -y

RUN go get -u github.com/jinzhu/gorm
RUN go get -u github.com/PuerkitoBio/goquery
RUN go get -u github.com/misterhex/azure-sdk-for-go/storage
RUN go get -u github.com/nu7hatch/gouuid

ADD . /go/src/bitbucket.org/misterhex/gomg

RUN cd /go/src/bitbucket.org/misterhex/gomg && go get .

RUN go install bitbucket.org/misterhex/gomg

ADD ./watermark.png /go/bin/

WORKDIR /go/bin/

ENTRYPOINT ./gomg

EXPOSE 3000
