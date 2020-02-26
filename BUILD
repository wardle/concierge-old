load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")
load("@bazel_gazelle//:def.bzl", "gazelle")
load("@rules_proto//proto:defs.bzl", "proto_library")

# gazelle:prefix github.com/wardle/concierge
gazelle(name = "gazelle")


go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "github.com/wardle/concierge",
    visibility = ["//visibility:private"],
    deps = ["//cmd:go_default_library"],
)

go_binary(
    name = "concierge",
    embed = [":go_default_library"],
    visibility = ["//visibility:public"],
)


