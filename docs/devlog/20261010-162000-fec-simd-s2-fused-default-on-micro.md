# S2融合编码生产默认选择：基于同runner微基准（仍需真实业务）

- 父 helper/source HEAD f6603f212420eee43bafdadaa6b98615f33177c7，冻结旧A=a2db258b436a41fdee98c6c53abec9bab6ce600f，当前 B=89fcb5e99ffc6ae63354ea6628d367ada72d6bed 尚为span默认。本提交修改产品代码后必须重新冻结全新B SOURCE SHA，不能继承旧B Actions 结果。
- 微基准 [Actions 38033888108](https://github.com/lly8666/wobuzhidao/actions/runs/38033888108) 成功：同一x86 runner单核、3次中位数，P20的96B span6481ns/fused3896ns，512B 10322ns/6208ns，1400B 27105ns/19987ns（内核耗时改善39.9%、39.9%、26.3%）。96/256/512/1000/1400 网络片长；标量vs SIMD 同host。附原始log artifact 11662308900。该数字不是业务CPU或p99，不可转移到ARM。
- 出于性能优先将完整20源 shard、native SIMD 支持时融合编码改为默认；partial1..19仍原active span，恢复仍复用 SIMD multiply；无ASM/未知硬件自动标量，显式 -tags=wbd_fec_span 恢复span，wbd_fec_scalar恢复原GF标量。适用为受控产品内部回退，无新GUI参数。保留32ms/3s/systematic原样。
- 首次真实pilot [Actions 38033888013](https://github.com/lly8666/wobuzhidao/actions/runs/38033888013) **FAIL**：A第一段异常PermissionError，B未运行，原始evidence artifact 11662823944。缺业务manifest/运行时flags，不能用于性能结论。需要权限修复和异常定位后再跑。
- 新默认产品的跨平台Actions单元/race与真实性能均 NOT_RUN（提交时）；旧默认B的S2 Actions不能冒充新B通过。旧历史Game4/80s S2C等FAIL/OPEN和physical NOT_RUN不变。
