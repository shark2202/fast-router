#!/usr/bin/env python3
"""Synthesize minimind SFT data for fast-router task-type routing.

Each conversation reproduces BYTE-EXACTLY the scorer's yes/no prompt:
  "{state}\nQuestion: Is this about \"{code}\" ({desc})? Answer Yes or No."
with assistant answer "Yes"/"No" — training the exact function we score.

Balanced sampling: 1 positive + 3 random negatives per state.
The 30 test-set messages are held out (exact-match filtered).
"""
import json, random, re, sys

random.seed(42)
DESC = json.load(open('/tmp/task_desc.json'))
CODES = sorted(DESC.keys())

# --- hold out the real test set ---
test_msgs = {s['message'] for s in json.load(open('/Users/mac/codes/fast-router/data/task_type_samples.json'))}

# --- per-category ingredient lists (Chinese-heavy, some English) ---
GEN = {
'A': dict(
    subjects=["用 Python","用 Go","用 Rust","用 TypeScript","用 bash","用 C++","用 Java","用 SQL"],
    verbs=["实现","写","开发","编写","构造","生成"],
    objects=["快速排序算法","LRU 缓存","二叉树遍历","HTTP 客户端","JSON 解析器","命令行工具","状态机","守护进程","单例模式","分页组件","登录表单","文件压缩脚本","正则提取器","内存池","协程调度器"],
    tails=["要求原地排序","支持并发","带单元测试","处理边界情况","含错误重试","性能优先","代码要有注释","兼容旧版本","","",""]),
'B': dict(
    openers=["这段代码","这个函数","下面的脚本","这份实现","这段逻辑","这个模块"],
    asks=["为什么返回空值","哪里有内存泄漏","帮我找出 bug","解释一下执行流程","为什么死锁了","review 一下有没有问题","这里的边界条件对吗","为什么编译不过","性能瓶颈在哪","逻辑有什么问题"],
    ctx=["","循环里的","","","并发场景下 ","递归调用处 ","异常处理后 "]),
'C': dict(
    qs=["分析","推导","论证","评估","比较","推理"],
    topics=["两个架构方案的取舍","这个决策的利弊","这段论证的逻辑漏洞","A 和 B 哪个更优","该假设是否成立","此结论的必要前提","反例是否存在","因果还是相关","最优策略是什么","风险如何量化"],
    tails=["给出理由","分点说明","用第一性原理","结合约束条件","","",""] ),
'D': dict(
    docs=["这份 50 页的季度报告","这篇 2 万字的论文","这本技术白皮书","这批客户反馈邮件","这份合同文本","这套 API 文档","这些会议纪要","这本产品手册","这份审计底稿","这组访谈记录"],
    asks=["总结核心观点","提炼关键结论","归纳主要论点","压缩成摘要","梳理时间线","提取行动项","按主题归类","找出风险条款","对比前后矛盾处","整理成表格"]),
'E': dict(
    heads=["从这封邮件抽取","从这段文本提取","把这份回复解析成","从下面的 JSON 抽取","从这段日志提取","把这份简历解析为","从这则公告抽取","把这段对话整理成"],
    fields=["发件人、日期和主题","所有金额和币种","姓名、职位和联系方式","订单号与状态","错误码和耗时","公司名与融资金额","时间地点人物","SKU 与库存数","版本号与变更项","症状与诊断"]),
'F': dict(
    kinds=["一篇","一首","一段","一个","一份"],
    works=["关于秋天的散文","科幻微小说","产品发布文案","武侠短篇","藏头诗","品牌 slogan","悼词","旅行随笔","广告脚本","童话故事","情书","歌词"],
    tails=["800 字","五言绝句","赛博朋克风","温暖治愈系","面向年轻人","商务正式","幽默一点","",""]),
'G': dict(
    medias=["这张截图","这张白板照片","这个 UI 设计稿","这张流程图","这张数据图表","这张架构图","这张手绘草图","这张票据照片"],
    asks=["描述里面的布局","识别其中的文字","说明图中的流程","对比左右两版差异","数一下节点数量","找出标注错误","转成文字描述","解释配色与层级"]),
'H': dict(
    acts=["部署到 staging 环境","跑一遍测试套件并修复失败用例","查一下今天的天气","搜索这个报错信息的解决方案","把这个目录备份到 S3","调用支付接口完成退款","抓取该页面的标题","给这个仓库提个 issue","执行迁移脚本","查询数据库当前连接数"],
    pre=["帮我","请","现在","立刻帮我","","",""]),
'I': dict(
    qs=["法国的首都是哪里","光速是多少","水的沸点是多少","Python 之父是谁","一年有多少天","长城有多长","TCP 默认端口是多少","奥运会几年一届","人体正常体温是多少","勾股定理公式是什么","珠穆朗玛峰多高","圆周率前10位是什么","地球绕太阳一圈多久","氧气占空气比例是多少","《红楼梦》作者是谁","HTTP 和 HTTPS 区别是什么","最小的质数是什么","一打是多少个","太阳系有几大行星","中国最长的河流是哪条","声音在空气中传播速度","RGB 里 R 代表什么","一小时多少秒","月亮绕地球一圈多久","人类第一次登月是哪年","万有引力是谁提出的","DNA 双螺旋是谁发现的","一英里等于多少公里","黄金分割率约是多少","计算机之父是谁","二进制里 1010 等于十进制几","地球上最大的海洋是哪个","蜂鸟会不会飞","维生素C 含量最高的水果之一","闪电的温度大概多高","埃菲尔铁塔在哪座城市","企鹅生活在北极还是南极","白金的化学符号是什么","一年有几个节气"]),
'J': dict(
    srcs=["把这段中文","将下面的英文","把这句日语","将这段法语","把这段韩文","将这条西班牙语","把这段德文","将这段俄文"],
    texts=["翻译成英文","翻译成中文","翻译成法语","译为日语","翻成西班牙语","翻译成德文","译为韩文","翻成俄文"]),
}

def make_state(code):
    g = GEN[code]
    if code == 'A':
        return f"{random.choice(g['subjects'])}{random.choice(g['verbs'])}一个{random.choice(g['objects'])}{random.choice(g['tails'])}".rstrip("，")
    if code == 'B':
        return f"{random.choice(g['openers'])}{random.choice(g['ctx'])}{random.choice(g['asks'])}"
    if code == 'C':
        return f"{random.choice(g['qs'])}{random.choice(g['topics'])}{random.choice(g['tails'])}"
    if code == 'D':
        return f"{random.choice(g['docs'])}，{random.choice(g['asks'])}"
    if code == 'E':
        return f"{random.choice(g['heads'])}{random.choice(g['fields'])}"
    if code == 'F':
        return f"写{random.choice(g['kinds'])}{random.choice(g['works'])}{random.choice(g['tails'])}"
    if code == 'G':
        return f"{random.choice(g['medias'])}，{random.choice(g['asks'])}"
    if code == 'H':
        return f"{random.choice(g['pre'])}{random.choice(g['acts'])}"
    if code == 'I':
        return random.choice(g['qs'])
    if code == 'J':
        return f"{random.choice(g['srcs'])}{random.choice(g['texts'])}"
    raise AssertionError(code)

PER_CAT = int(sys.argv[1]) if len(sys.argv) > 1 else 250
convs = []
seen = set()
for code in CODES:
    made = 0
    tries = 0
    while made < PER_CAT and tries < PER_CAT * 40:
        tries += 1
        s = make_state(code)
        if s in seen or s in test_msgs or len(s) < 6:
            continue
        seen.add(s)
        made += 1
        # positive
        convs.append({"conversations": [
            {"role": "user", "content": f"{s}\nQuestion: Is this about \"{code}\" ({DESC[code]})? Answer Yes or No."},
            {"role": "assistant", "content": "Yes"}]})
        # 3 random negatives
        for nc in random.sample([c for c in CODES if c != code], 3):
            convs.append({"conversations": [
                {"role": "user", "content": f"{s}\nQuestion: Is this about \"{nc}\" ({DESC[nc]})? Answer Yes or No."},
                {"role": "assistant", "content": "No"}]})

random.shuffle(convs)
out = '/tmp/fr-distill/sft_router.jsonl'
import os; os.makedirs('/tmp/fr-distill', exist_ok=True)
with open(out, 'w') as f:
    for c in convs:
        f.write(json.dumps(c, ensure_ascii=False) + '\n')
pos = sum(1 for c in convs if c['conversations'][1]['content'] == 'Yes')
print(f"{len(convs)} conversations ({pos} Yes / {len(convs)-pos} No), {len(seen)} unique states -> {out}")
