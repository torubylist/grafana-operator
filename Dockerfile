FROM 172.16.1.99/gold/alpine
MAINTAINER Tristan Sloughter <t@crashfast.com>

RUN addgroup -S kube-operator && adduser -S -g kube-operator kube-operator

USER kube-operator

COPY ./bin/grafana-dashboard-operator .


ENTRYPOINT ["./grafana-dashboard-operator"]
