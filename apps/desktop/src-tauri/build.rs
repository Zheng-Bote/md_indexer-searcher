// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

fn main() {
    // generate_context! embeds the icons only when the crate is recompiled; rerunning the build
    // script on icon changes forces that, otherwise a new icon keeps the old one in dev builds.
    println!("cargo:rerun-if-changed=icons");
    tauri_build::build()
}
