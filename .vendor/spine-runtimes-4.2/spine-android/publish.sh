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
# - Edit spine-libgdx/gradle.properties and update the 'version' property. YES, THIS IS THE SINGLE SOURCE OF TRUTH.
# - For releases: version=4.2.9 (no -SNAPSHOT suffix)
# - For snapshots: version=4.2.9-SNAPSHOT (with -SNAPSHOT suffix)
#
# Usage: ./publish.sh
# The script automatically detects snapshot vs release based on version in gradle.properties
#

set -e

# Read version from spine-libgdx gradle.properties (single source of truth)
VERSION=$(grep '^version=' ../spine-libgdx/gradle.properties | cut -d'=' -f2)

if echo "$VERSION" | grep -q "SNAPSHOT"; then
    echo "Publishing SNAPSHOT version $VERSION to Central Portal..."
    ./gradlew :spine-android:publishReleasePublicationToSonaTypeRepository --info
else
    echo "Publishing RELEASE version $VERSION to Central Portal via JReleaser..."
    ./gradlew :spine-android:publishReleasePublicationToSonaTypeRepository -PRELEASE
    ./gradlew :spine-android:jreleaserDeploy -PRELEASE --info
fi