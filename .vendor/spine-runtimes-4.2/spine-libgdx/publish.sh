#!/bin/sh

#
# Modern Central Portal Publishing Setup:
# 1. Set up PGP key for signing: gpg --generate-key
# 2. Create Central Portal account at https://central.sonatype.com/
# 3. Generate user token in Central Portal settings
# 4. Create ~/.gradle/gradle.properties with:
#    MAVEN_USERNAME=<central-portal-username>
#    MAVEN_PASSWORD=<central-portal-token>
#    signing.gnupg.passphrase=<pgp-key-passphrase>
#
# Version Configuration:
# - Edit gradle.properties and update the 'version' property
# - For releases: version=4.2.9 (no -SNAPSHOT suffix)  
# - For snapshots: version=4.2.9-SNAPSHOT (with -SNAPSHOT suffix)
#
# Usage: ./publish.sh
# The script automatically detects snapshot vs release based on version in gradle.properties
#

set -e

# Read version from gradle.properties
VERSION=$(grep '^version=' gradle.properties | cut -d'=' -f2)

if echo "$VERSION" | grep -q "SNAPSHOT"; then
    echo "Publishing SNAPSHOT version $VERSION to Central Portal..."
    ./gradlew publishReleasePublicationToSonaTypeRepository
else
    echo "Publishing RELEASE version $VERSION to Central Portal via JReleaser..."
    ./gradlew publishReleasePublicationToSonaTypeRepository -PRELEASE
    ./gradlew jreleaserDeploy -PRELEASE
fi