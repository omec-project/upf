FROM python:2.7-slim as pip
RUN apt-get update && apt-get install -y gcc
RUN pip install --no-cache-dir psutil pyroute2

# https://gist.github.com/krsna1729/c4862f278e74b337177937b6e70cc4a2
FROM krsna1729/bess
COPY --from=pip /usr/local/lib/python2.7/site-packages/psutil /usr/local/lib/python2.7/site-packages/psutil
COPY --from=pip /usr/local/lib/python2.7/site-packages/pyroute2 /usr/local/lib/python2.7/site-packages/pyroute2
COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
