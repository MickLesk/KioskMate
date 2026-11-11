# Release checklist

1. **Changelog**: `CHANGELOG.md` has a non-empty `## vX.Y.Z` section for the
   version (pre-releases: `## v1.0.0-pre.N`). Check it:
   `bash scripts/check-changelog.sh vX.Y.Z`. The release workflow stops at once
   without it; 1.0.0-pre.3 failed this way.
2. **Main is green**: CI (tests, Windows, e2e normal and minified).
3. **Image packages** (when `image/` changed): the "Image packages" workflow
   is green.
4. **Tag the merge commit on main**:
   ```bash
   git checkout main && git pull
   git tag vX.Y.Z && git push origin vX.Y.Z
   ```
   Tags containing `pre`, `alpha`, `beta` or `rc` are published as pre-releases; the
   updater offers them only with "Offer pre-releases" switched on.
5. **Release workflow** builds the deb packages (arm64, amd64), Windows zips
   and checksums and uploads them. If a release with that tag already exists,
   it uploads into it instead of failing.
6. **If the workflow failed**: fix on main, then move the tag:
   ```bash
   git tag -d vX.Y.Z && git push origin :refs/tags/vX.Y.Z
   git tag vX.Y.Z && git push origin vX.Y.Z
   ```
7. **Optional image**: run the "Raspberry Pi image" workflow manually with the
   version and attach the image to the release.
8. **Smoke test on a Pi**: update from the previous version via the Admin UI,
   check Overview, Browser Doctor and touch.
