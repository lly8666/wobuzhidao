# 20261007-224800 MTU9000修复前fixture过滤修正

预修复诊断SOURCE `a3ce2f552b04776dbcc2654b9fcef54c7cc3edb8` 的 next-predelivery-tools run `37638831165` 保留为FAIL。失败job中既有 raw-send blackhole fixture先PASS，既有reverse fragment fixture也以56 fragments / observer drop0 PASS；新增 `test_inner_mtu9000_fragment_kernel.py` 在第一条8972探针读取TUN时遇到非IPv4/非目标帧，脚本直接AssertionError。这个失败发生在目标UDP片形状判定之前，不能当成9000边界失败或产品失败。

本提交只修资格fixture：与既有 `physical_inner_fragment_watch.py` 一致，对非IPv4、非受控source/destination、非UDP帧返回None并忽略；在识别当前sequence的首片前也忽略其他目标流，识别后只接受同一IP ID。没有改raw socket、runtimeowner、LINK/FEC、MTU、RTO、FEC档位、4096或任何正式入口参数。

下一步重新跑同一预修复诊断。旧FAIL不删除。只有真实Linux TUN MTU9000的8972/8973/65507/65508+DF边界和Go层 sparse-RTO/LINK-FEC no-HOL都通过，才进入算法修复。
