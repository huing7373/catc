import Foundation

/// 传感器层到状态机的输入契约
/// Story 1.2 的 SensorManager 将 CMMotionActivity 转换为 MotionInput 后传给 CatStateMachine
public enum MotionInput: String, Codable, Sendable {
    case stationary
    case walking
    case running
    case wristRaise
}
