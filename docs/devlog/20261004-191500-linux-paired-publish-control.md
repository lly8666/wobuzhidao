# 20261004-191500 Linux/Windows配套预发布

固定产品SOURCE c956481143857d6ae4cdce04a31d22e8f6438903，所有请求中的原始attempt1同源门成功：foundation37197185706、GUI37197185741、native部署37197185836、lifecycle core37197185786、36+aggregate37197185759、steady37197185762。独立Normal5205 37197508391及Game5205 37197510919成功，只读审计37197288840成功；其artifact11301517648 SHA256 15d2c98f7081ca3b2ccd8697c2039ecac51a5b95805b8b4d53f517104fb79b64已下载核验。

Normal双向约10Mbps，stress byte loss 0/0.001600%，p95 614.08ms；Game约3Mbps、loss0、p95 602.35ms；两者socket drop0，环境和正确性门均PASS。CPU跨host不宣称固定下降：Normal48.63/49.04 CPU-s，Game94.72/90.26 CPU-s，120s样本。

发布同SOURCE三份包，不替代985历史标签，明确ARM cross/Windows物理/full70/full18/1800s仍NOT_RUN；公开notes及包内安装文档说明Linux管理与自动7天内存租约。发布控制只核验产物并发布，不额外运行性能。完成后核验GitHub资产实际哈希，再更新产品交接证据。
