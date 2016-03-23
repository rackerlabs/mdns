FROM mysql:5.6

ENV MYSQL_ROOT_PASSWORD password
ENV MYSQL_DATABASE designate

COPY designate.sql /docker-entrypoint-initdb.d/designate.sql
COPY customconfig.cnf /etc/mysql/conf.d/customconfig.cnf
