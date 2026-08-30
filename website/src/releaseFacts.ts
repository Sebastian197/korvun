// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// releaseFacts: the ONLY place under src/ where a release version is
// spelled. Every component interpolates from here, and
// scripts/check-release-facts.mjs fails the build on any literal
// elsewhere — the root fix for the 2026-08-30 public-truth find, where
// the hero, the install block and the release-notes href served
// v0.11.0 while the copy said v0.13.0 (four hand-updated spots, one
// updated).
export const releaseFacts = {
  /** The current public release, bare (install VERSION= lines). */
  version: "0.13.0",
  /** The current public release, tagged (hrefs, visible copy). */
  tag: "v0.13.0",
  /**
   * The release the landing demo video was recorded against — a fact
   * about the recorded artifact, which moves only when the video is
   * re-recorded, never with the current release.
   */
  demoTag: "v0.9.0",
} as const;
