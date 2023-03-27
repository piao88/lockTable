# lockTable

根据题目描述，实现一个简单的，支持读写锁，死锁检测的lockTable



## 描述

lockTable中的单元是lockState，其中主要分为holder和waitQueue两种角色，holder是锁占有者，由于读锁可以共享，所以是个array。waitQueue中按照进入锁表的先后顺序排队。

tryActiveWait（）：需要等待的事务进入waitQueue，并且将依赖关系写入Dependence，无需等待的情况则直接返回false。

waitOnAndDeadLockDetect（）：等待holder或队列前面的事务结束，等待过程中定时查询死锁。

AcquireLock（）：成为锁占有者，并将请求从waitQueue中移出，更新死锁依赖关系，注意读锁升级成xLock的情况.

releaseLock（）：事务结束，释放锁，通知waitQueue的第一个等待者，结束等待成为holder。其中从sharedLock Holder释放锁的情况需要多加注意，比如txnA和txnB共享holder，txnA释放了，txnB独占holder时需要释放waitQueue中txnB的其他request。



worker（）：原子执行三个读一个写，流程如下：

```go
sequence()
acquireLock()
// execute read or write
releaseLock()
```



## TODO LIST：

死锁检测目前在检测到死锁后，还不具备从lockTable中自动移出的流程，目前只能返回error。

工作繁忙，未能尽善尽美，望见谅。

