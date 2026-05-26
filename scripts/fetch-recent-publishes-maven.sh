#!/usr/bin/env bash
set -euo pipefail

# Fetches recently updated Maven Central packages for supply chain scanning.
# Output: one line per package: "group:artifact new_version previous_version"
#
# Uses Maven Central Search API.

MAX_PACKAGES=${1:-25}

echo "# Fetching recently updated Maven Central packages" >&2
echo "# Maximum packages to scan: ${MAX_PACKAGES}" >&2

# High-value Java/Kotlin packages to monitor
# Format: "groupId:artifactId"
WATCHLIST=(
  "org.apache.logging.log4j:log4j-core"
  "org.apache.logging.log4j:log4j-api"
  "org.springframework:spring-core"
  "org.springframework.boot:spring-boot"
  "org.springframework.security:spring-security-core"
  "com.google.guava:guava"
  "com.fasterxml.jackson.core:jackson-databind"
  "org.apache.kafka:kafka-clients"
  "io.netty:netty-handler"
  "com.squareup.okhttp3:okhttp"
  "org.apache.httpcomponents.client5:httpclient5"
  "io.grpc:grpc-core"
  "com.amazonaws:aws-java-sdk-core"
  "software.amazon.awssdk:s3"
  "org.elasticsearch.client:elasticsearch-rest-high-level-client"
  "org.opensearch.client:opensearch-java"
  "com.stripe:stripe-java"
  "io.jsonwebtoken:jjwt-api"
  "org.bouncycastle:bcprov-jdk18on"
  "com.auth0:java-jwt"
  "org.apache.commons:commons-lang3"
  "commons-io:commons-io"
  "org.slf4j:slf4j-api"
  "ch.qos.logback:logback-classic"
  "org.jetbrains.kotlin:kotlin-stdlib"
)

count=0
for ga in "${WATCHLIST[@]}"; do
  if [ $count -ge $MAX_PACKAGES ]; then
    break
  fi

  GROUP=$(echo "$ga" | cut -d: -f1)
  ARTIFACT=$(echo "$ga" | cut -d: -f2)

  # Query Maven Central Search API for latest versions
  versions=$(curl -s "https://search.maven.org/solrsearch/select?q=g:${GROUP}+AND+a:${ARTIFACT}&rows=2&wt=json&core=gav" 2>/dev/null | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
    docs = d.get('response', {}).get('docs', [])
    if len(docs) >= 2:
        print(f'{docs[0][\"v\"]} {docs[1][\"v\"]}')
    elif len(docs) == 1:
        print(f'{docs[0][\"v\"]} {docs[0][\"v\"]}')
except:
    pass
" 2>/dev/null || echo "")

  if [ -n "$versions" ]; then
    new_ver=$(echo "$versions" | awk '{print $1}')
    prev_ver=$(echo "$versions" | awk '{print $2}')
    if [ "$new_ver" != "$prev_ver" ]; then
      echo "$ga $new_ver $prev_ver"
      count=$((count + 1))
    fi
  fi
done

echo "# Found $count Maven packages to scan" >&2
