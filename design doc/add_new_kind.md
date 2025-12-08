To implement a new node type:
1. implement `common.System` interface
2. add new function to `common.NewSysRegistry[]` in `init()`
3. add its type in `OneOfSystem` struct