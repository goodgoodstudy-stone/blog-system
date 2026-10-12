package seed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"blog-system/backend/internal/auth"
)

// Extra adds optional, repeatable local demo content. The normal migration path
// does not call this function, so release deployments keep their smaller seed.
func Extra(ctx context.Context, db *sql.DB, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	author1, err := user(ctx, db, "author1@blog.local", "林间来信", "author", hash)
	if err != nil {
		return err
	}
	author2, err := user(ctx, db, "author2@blog.local", "午后手记", "author", hash)
	if err != nil {
		return err
	}
	reader1, err := user(ctx, db, "reader1@blog.local", "山茶", "reader", hash)
	if err != nil {
		return err
	}
	reader2, err := user(ctx, db, "reader2@blog.local", "晴窗", "reader", hash)
	if err != nil {
		return err
	}

	tagIDs := make(map[string]int64)
	for _, name := range []string{"工程实践", "生活记录", "可观测性", "数据库", "后端开发", "阅读笔记", "旅行", "产品设计", "写作", "摄影"} {
		id, err := tag(ctx, db, name)
		if err != nil {
			return err
		}
		tagIDs[name] = id
	}

	comments := []string{
		"这里的步骤很清楚，我准备按文中的顺序试一次。",
		"这个例子帮我理解了取舍，期待后续更新。",
		"读完之后回头检查了自己的项目，确实发现了类似问题。",
		"把过程和结果放在一起看，比只看结论更有帮助。",
		"我喜欢这段具体的经验，已经收藏起来了。",
	}
	for i, fixture := range extraArticles {
		author := author1
		if fixture.author == 2 {
			author = author2
		}
		id, err := ensureExtraArticle(ctx, db, author, fixture, i)
		if err != nil {
			return fmt.Errorf("article %q: %w", fixture.title, err)
		}
		for _, name := range fixture.tags {
			if _, err := db.ExecContext(ctx, `INSERT IGNORE INTO article_tags(article_id,tag_id) VALUES(?,?)`, id, tagIDs[name]); err != nil {
				return fmt.Errorf("tags for %q: %w", fixture.title, err)
			}
		}
		if fixture.status != "published" {
			continue
		}
		commenter := reader1
		if i%2 == 1 {
			commenter = reader2
		}
		if err := ensureExtraComment(ctx, db, id, commenter, comments[i%len(comments)]); err != nil {
			return err
		}
		if i%3 == 0 {
			if err := ensureExtraComment(ctx, db, id, reader2, "补充一点：这个主题值得在实际项目里持续观察。"); err != nil {
				return err
			}
		}
		if i%2 == 0 {
			if _, err := db.ExecContext(ctx, `INSERT IGNORE INTO favorites(user_id,article_id) VALUES(?,?)`, reader1, id); err != nil {
				return err
			}
		}
	}
	return nil
}

type extraArticle struct {
	author                         int
	title, summary, detail, status string
	tags                           []string
}

var extraArticles = []extraArticle{
	{1, "一次请求如何穿过博客后端", "从入口中间件到数据库查询，沿着一条请求理解服务边界。", "先在入口生成请求 ID，再按路由记录耗时。业务层负责规则，存储层负责查询；当请求变慢时，逐层比较时间就能缩小范围。", "published", []string{"工程实践", "后端开发"}},
	{2, "清晨写作的二十分钟", "给每天留一小段不受打扰的写作时间。", "我会先写下今天看到的一件小事，再补充它带来的感受。二十分钟结束就停笔，隔天再整理，反而更容易坚持。", "published", []string{"生活记录", "写作"}},
	{1, "从日志找到一条慢请求", "用请求 ID、Trace 和数据库日志还原一次完整调用。", "先按时间和接口筛选错误日志，再点击 trace ID 查看 Span 瀑布图。数据库 Span 的耗时如果明显偏高，就继续核对 SQL 与结果行数。", "published", []string{"可观测性", "后端开发"}},
	{2, "周末城市散步路线", "避开热门景点，记录一条适合慢慢走的路线。", "从旧书店出发，穿过安静的街巷，最后在河边坐一会儿。路线不必排满，给临时发现的店和风景留出时间。", "published", []string{"旅行", "摄影"}},
	{1, "数据库连接池为什么会等待", "把连接数、并发请求和等待时间放在一起看。", "连接池上限决定同一时刻能有多少数据库操作执行。若使用中连接长期接近上限，先检查慢查询和事务持有时间，再决定是否增加连接数。", "published", []string{"数据库", "后端开发"}},
	{2, "一本书读到一半时", "暂时读不下去，也可以留下有用的笔记。", "我会记下已经理解的章节和仍有疑问的地方。过一段时间再读，往往能看出上次忽略的线索。", "published", []string{"阅读笔记", "生活记录"}},
	{1, "给 API 设计可用的错误信息", "错误码、请求 ID 和提示文案各有职责。", "客户端需要稳定的错误码，用户需要能采取行动的提示，排查人员需要请求 ID。把内部异常原文直接返回给用户通常没有帮助。", "published", []string{"产品设计", "后端开发"}},
	{2, "雨后拍照的光线", "阴天的柔和光线适合观察颜色和细节。", "雨后的路面会反射天空，街角的植物也更鲜亮。我通常先观察光从哪里来，再决定画面里保留多少背景。", "published", []string{"摄影", "生活记录"}},
	{1, "Prometheus 面板里的空值", "没有请求时，延迟分位数可能没有有效样本。", "QPS 为零和采集故障不是一回事。先看抓取状态，再看请求计数是否变化；没有样本时应明确显示无请求，避免误读为零毫秒。", "published", []string{"可观测性", "工程实践"}},
	{2, "把房间整理成能读书的样子", "从桌面和灯光开始，给注意力留出空间。", "我只留下正在读的书和一支笔，其他东西放回固定位置。环境简单一点，坐下后就更容易进入阅读状态。", "published", []string{"阅读笔记", "生活记录"}},
	{1, "分页查询的边界条件", "总数、页码和排序字段需要一起设计。", "固定排序键能避免翻页时内容跳动。查询条件必须同时用于总数和列表，并给 pageSize 设置上限，防止一次请求拉取过多记录。", "published", []string{"数据库", "工程实践"}},
	{2, "去海边时带的一本薄书", "旅行与阅读有时会改变彼此的节奏。", "火车上读过的几页，回家后仍能让我想起那天的天气。旅行里不一定读得多，但记忆会把文字和地方连在一起。", "published", []string{"旅行", "阅读笔记"}},
	{1, "如何读一张 Trace 瀑布图", "先找到长 Span，再辨别等待、计算和数据库访问。", "瀑布图展示的是时间关系，不代表每个 Span 都值得优化。先看父子关系，再结合日志和指标判断原因，避免只凭最长的一条线下结论。", "published", []string{"可观测性", "工程实践"}},
	{2, "给文章配图的三个原则", "让图片补充信息，而不是打断阅读。", "配图应与段落内容相关，并保持清晰的替代文本。尺寸和裁切要照顾手机屏幕，加载时也不要让正文突然跳动。", "published", []string{"摄影", "产品设计"}},
	{1, "慢查询排查笔记", "先确认执行频率，再看索引和返回行数。", "单次慢查询和高频中等耗时查询都可能拖累服务。对照执行计划、筛选条件和实际数据分布，再决定改索引还是改查询方式。", "published", []string{"数据库", "可观测性"}},
	{2, "写一篇值得重读的日记", "只记录事件还不够，补上当时的判断和疑问。", "过几个月回看时，最有价值的往往不是做了什么，而是为什么做。写下当时的选择，未来才能看出自己如何改变。", "published", []string{"写作", "生活记录"}},
	{1, "从健康检查到真正可用", "进程存活、数据库就绪和业务成功是不同层次。", "存活检查只需要证明进程能响应；就绪检查还要确认关键依赖可访问。业务指标则告诉我们用户请求是否真的成功。", "published", []string{"可观测性", "后端开发"}},
	{2, "小城夜晚的步行地图", "记录熟悉街道在夜里呈现出的另一种样子。", "商店关门后，招牌、路灯和行人的声音会变得突出。慢慢走一圈，能发现白天总被忽略的小细节。", "published", []string{"旅行", "摄影"}},
	{1, "文章搜索如何验证", "用同义词、单字和不存在的词检查搜索体验。", "除了命中标题，还要检验摘要和正文。搜索结果应只显示已发布文章，并与标签筛选、分页保持一致。", "published", []string{"数据库", "产品设计"}},
	{2, "读书笔记不用写成书评", "保存自己的疑问，比复述整本书更有意义。", "我会挑出一两段真正触动我的内容，写下同意或反对的理由。这样的笔记更短，却更容易在以后派上用场。", "published", []string{"阅读笔记", "写作"}},
	{1, "一次发布流程的检查清单", "从草稿、预览到公开访问，逐步确认状态。", "发布前检查标题、摘要、标签、图片和链接。发布后再用未登录视角访问文章，确认权限与缓存都符合预期。", "published", []string{"工程实践", "产品设计"}},
	{2, "午后给自己留一点空白", "没有安排的时间，也可以成为生活的一部分。", "关掉通知，泡一杯茶，看看窗外的光。空白不是效率的反面，它让下一件事开始时更从容。", "published", []string{"生活记录", "写作"}},
	{1, "待完善：数据库索引实验", "草稿示例，供作者在管理页继续编辑。", "先准备不同数据量，再比较索引前后的执行计划与实际耗时。实验结果整理后再发布。", "draft", []string{"数据库", "工程实践"}},
	{2, "暂停发布的旅行手记", "下线状态示例，仅作者在管理页可见。", "这篇手记暂时不公开，后续补齐地点和照片后再决定是否重新发布。", "unpublished", []string{"旅行", "写作"}},
}

func ensureExtraArticle(ctx context.Context, db *sql.DB, author int64, a extraArticle, index int) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, `SELECT id FROM articles WHERE author_id=? AND title=? LIMIT 1`, author, a.title).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	body := fmt.Sprintf("# %s\n\n%s\n\n## 记录\n\n%s", a.title, a.summary, a.detail)
	createdAt := time.Now().UTC()
	var publishedAt any
	if a.status == "published" {
		createdAt = createdAt.Add(-time.Duration(len(extraArticles)-index) * 24 * time.Hour)
		publishedAt = createdAt.Add(2 * time.Hour)
	}
	r, err := db.ExecContext(ctx, `INSERT INTO articles(author_id,title,summary,body_md,body_plain,status,published_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, author, a.title, a.summary, body, body, a.status, publishedAt, createdAt, createdAt)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func ensureExtraComment(ctx context.Context, db *sql.DB, articleID, userID int64, content string) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM comments WHERE article_id=? AND user_id=? AND content=?`, articleID, userID, content).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `INSERT INTO comments(article_id,user_id,content) VALUES(?,?,?)`, articleID, userID, content)
	return err
}
