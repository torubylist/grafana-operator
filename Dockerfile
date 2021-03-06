FROM 172.16.1.99/gold/alpine
MAINTAINER  Yongping Zhao

RUN addgroup -S kube-operator && adduser -S -g kube-operator kube-operator

USER kube-operator

COPY ./bin/grafana-dashboard-watcher .


ENTRYPOINT ["./grafana-dashboard-watcher"]
