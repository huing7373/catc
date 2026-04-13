import Foundation

/// 最小接入点：保留一个统一入口，后续如果需要再次接入 Spine 可继续扩展。
public enum SpineBootstrap {
    public static func runtimeVersionDescription() -> String {
        "Spine runtime integration deferred"
    }
}
