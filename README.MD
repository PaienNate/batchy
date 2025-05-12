# 测试结果
=== RUN   TestChanBatcherPerformance

chan_batcher_test.go:113: 总项目数: 3000000

chan_batcher_test.go:114: 处理时间: 16.1339097s

chan_batcher_test.go:115: 每秒处理项目数: 185943.77

chan_batcher_test.go:116: 数据库记录数: 3000000

测试完成，总数据量: 3000000, 总时间: 16.1339097s, 每秒处理量: 185943.77, 数据库记录数: 3000000

--- PASS: TestChanBatcherPerformance (18.06s)

PASS

=== RUN   TestBatcherPerformance
batcher_test.go:115: 总项目数: 3000000

batcher_test.go:116: 处理时间: 15.6434011s

batcher_test.go:117: 每秒处理项目数: 191774.15

batcher_test.go:118: 数据库记录数: 3000000

测试完成，总数据量: 3000000, 总时间: 15.6434011s, 每秒处理量: 191774.15, 数据库记录数: 3000000

--- PASS: TestBatcherPerformance (17.66s)
PASS

示例代码见TEST文件

