FROM ubuntu:18.04

# Install nginx, vim (optional), curl (optional) & CertBot
RUN apt update -y \
    && apt install nginx vim curl -y \
    && apt-get install software-properties-common -y \
    && add-apt-repository ppa:certbot/certbot -y \
    && apt-get update -y \
    && apt-get install python-certbot-nginx -y \
    && apt-get clean


# Expose standard HTTP/HTTPS ports
EXPOSE 80/tcp
EXPOSE 443/tcp

STOPSIGNAL SIGTERM

# Run Nginx without halting
CMD ["nginx", "-g", "daemon off;"]