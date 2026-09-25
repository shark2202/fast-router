// build.zig — build libfrwrapper.{dylib,so,dll} linking llama.cpp shared libs.
//
// Cross-compile: zig build -Dtarget=aarch64-linux-gnu -Dllama_dir=/path/to/linux-libs
// (simplified: avoids getEnvVarOwned/addRPath that crashed zig 0.14 build runner)
const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const lib = b.addSharedLibrary(.{
        .name = "frwrapper",
        .root_source_file = b.path("frwrapper.zig"),
        .target = target,
        .optimize = optimize,
    });

    lib.addIncludePath(b.path("include"));

    // llama.cpp shared lib dir: build option -Dllama_dir=..., default macos-x64 unpack.
    const llama_dir = b.option([]const u8, "llama_dir", "path to unpacked llama.cpp libs") orelse
        "/tmp/llama-bins/llama-b11175";
    lib.addLibraryPath(.{ .cwd_relative = llama_dir });
    lib.linkSystemLibrary("llama");
    lib.linkSystemLibrary("ggml-base");
    lib.linkSystemLibrary("ggml-cpu");

    b.installArtifact(lib);
}
