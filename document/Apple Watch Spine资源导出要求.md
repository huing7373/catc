# 裤衩猫 Apple Watch Spine 资源导出要求

## 目的

这份文档是给美术和 Spine 导出同学用的。

目标只有一个：

- 导出一套 **能被当前 Apple Watch 客户端直接播放** 的 Spine 资源

当前 Watch 端并不是用官方 `spine-ios` 运行时在渲染，而是使用基于 `SpriteKit` 的轻量运行时。
这意味着资源格式必须满足当前运行时的能力边界，不能只看 Spine 编辑器里能不能播。

---

## 当前 Watch 端的真实限制

### 1. 只可靠支持 `region` 附件

当前 Watch 运行时虽然能解析 `mesh` 数据，但真正创建显示节点时只实现了 `RegionAttachment`。

结论：

- 可以用：`region`
- 不要用：`mesh`
- 不要用：`linkedmesh`
- 不建议依赖：`clipping`
- 不建议依赖：`path`

### 2. 当前猫资源失败的根因

当前这份 [cat.json](/Users/boommice/catc/ios/CatWatch/Resources/Art/Spine/Cat/cat.json) 里的 attachment 全部是 `mesh`。

所以问题不是：

- JSON 是不是图集导出
- 图片是不是散图

而是：

- **attachment 类型不兼容当前 Watch 运行时**

这也是为什么现在会出现：

- 有骨骼
- 有动画数据
- 但画面不正常或不播放

---

## 导出目标格式

请导出为：

- `JSON`
- `单张散图 PNG`
- 每个 attachment 对应一张图片
- attachment 类型为 `region`

推荐目录结构：

```text
ios/CatWatch/Resources/Art/Spine/Cat/
├── cat.json
└── images/
    ├── body.png
    ├── head.png
    ├── tail.png
    ├── leg_front_left.png
    ├── leg_front_right.png
    └── ...
```

客户端这边会再把这些图片组织成 SpriteKit 可读的 atlas 目录，因此导出阶段不用强依赖官方 `.atlas` 文本。

---

## 必须满足的导出要求

### 1. attachment 必须是 `region`

这是最重要的一条。

导出后的 `cat.json` 里，每个图片 attachment 应该更接近下面这种结构：

```json
"head": {
  "head": {
    "x": 12,
    "y": 34,
    "rotation": 0,
    "width": 123,
    "height": 118
  }
}
```

而不是下面这种 `mesh` 结构：

```json
"head": {
  "head": {
    "type": "mesh",
    "uvs": [...],
    "triangles": [...],
    "vertices": [...]
  }
}
```

### 2. 动画名称固定为四个

请保持这四个状态名：

- `idle`
- `walking`
- `running`
- `sleeping`

不要再出现下面这些变体：

- `idel`
- `walk`
- `run`
- `sleep`

因为 Watch 端当前就是按这四个名字做状态映射。

### 3. 图片命名要与 attachment 名一致

建议 attachment 名和图片文件名一一对应，例如：

- `body` -> `body.png`
- `head` -> `head.png`
- `tail` -> `tail.png`

这样客户端最稳定，不需要额外做名字映射。

### 4. 尽量不要带额外 path 层级

最稳妥的方式是让 attachment 直接走默认贴图组，不要写复杂路径。

推荐：

- `name = head`
- `fileName = head`
- 或者根本不写 `path`

不推荐：

- `path = images/head`
- `path = cat/images/head`
- 多层 skin atlas 路径混用

原因是当前 Watch 端的 atlas 查找逻辑比较轻量，路径层级越复杂，越容易在运行时找不到贴图。

### 5. 所有图片都要是透明 PNG

要求：

- 背景透明
- 非 JPG
- 不混入无关图片

例如之前目录里混入过一张微信图片，这种图会被一起打进 atlas，导致包体变大、资源污染。

### 6. 不要在 Watch 首版资源里用变形网格效果

为了兼容当前运行时，以下能力暂时都不要依赖：

- FFD / deform
- mesh 扭曲
- linked mesh
- clipping mask

首版 Watch 猫动画建议只靠：

- bone transform
- slot attachment 切换
- region 图片替换

来完成。

---

## 建议的骨骼与动画制作方式

### 推荐做法

- 身体、头、尾巴、四肢都使用普通 region 图片
- 通过骨骼旋转、位移、缩放来做动作
- 通过 slot attachment 切换来表现不同腿型、眼睛、尾巴状态

### 不推荐做法

- 脸部红晕、耳朵、身体全部做成 mesh
- 依赖大量顶点形变来做呼吸、走路、睡觉

原因很简单：

- Spine 编辑器里这样做很灵活
- 但当前 Watch runtime 画不出来

---

## 给导出同学的检查清单

导出前请逐项确认：

- attachment 类型不是 `mesh`
- attachment 类型不是 `linkedmesh`
- 动画名是 `idle / walking / running / sleeping`
- 每个图片 attachment 都有对应 PNG
- PNG 文件名与 attachment 名一致
- 没有混入无关图片
- 没有使用 deform / clipping / path 作为关键表现手段
- 默认 skin 名保持 `default`

---

## 可接受的最小资源标准

如果要先快速验证播放，最小可接受版本如下：

- 只有一个 skin：`default`
- 四个动画：
  - `idle`
  - `walking`
  - `running`
  - `sleeping`
- 所有 attachment 都是 `region`
- 图片放在 `images/` 目录

只要满足这个最低标准，客户端就可以先接起来验证。

---

## 当前项目最推荐的导出方式

对裤衩猫现在这版 Apple Watch 来说，最推荐的是：

1. 在 Spine 里继续做骨骼动画
2. 但导出时确保 attachment 是 `region`
3. 每个 attachment 对应单独 PNG
4. 保持四个标准状态名
5. 用 `default` skin 先跑通

一句话总结：

**请把猫资源导出成“region + 散图 PNG + 标准状态名”的 JSON，不要导出成 mesh 版 JSON。**

---

## 如果一定要保留 mesh，后续怎么办

如果后面你们坚持要保留 mesh 变形能力，也不是完全不行，但那是下一阶段工作：

- 要么扩展当前 Watch runtime，补齐 mesh 渲染
- 要么换成真正支持 mesh 的运行时方案

这不是简单改一个 JSON 能解决的事。

所以当前阶段请优先按本文档导出兼容版资源。
