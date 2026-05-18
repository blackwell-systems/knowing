import {makeScene2D, Circle, Line, Txt, Rect, Node} from '@motion-canvas/2d';
import {
  all,
  waitFor,
  createRef,
  createRefArray,
  Vector2,
  sequence,
  easeOutCubic,
  easeInOutCubic,
} from '@motion-canvas/core';

/**
 * "Graph Produces Context" — Force-directed style
 *
 * Mimics the knowing-viz aesthetic: clustered nodes, community colors,
 * thin edges, organic layout. Shows a query entering, RWR ripple spreading,
 * relevant nodes growing, irrelevant fading, top-K lifting out into
 * a ranked context list.
 *
 * The point: the graph doesn't just store relationships, it RANKS them
 * and produces agent-ready context in one call.
 */
export default makeScene2D(function* (view) {
  const bg = '#08080c';
  const text = '#e8e8f0';
  const muted = '#5a6a7a';
  const accent = '#4ecdc4';
  const edgeColor = '#1a1a2a';

  // Community palette (matches knowing-viz)
  const communities = [
    '#4ecdc4', // context engine (teal)
    '#ff6b6b', // MCP server (red)
    '#feca57', // indexer (yellow)
    '#a29bfe', // store (purple)
    '#fd79a8', // wire format (pink)
  ];

  view.fill(bg);

  // Pre-computed node positions mimicking ForceAtlas2 clusters
  // Each cluster is positioned in a different area with internal scatter
  const nodeData: {x: number; y: number; community: number; name: string; relevance: number}[] = [];

  // Community 0: Context engine (center-left) — spread for no overlap at scale
  const c0 = [
    {x: -200, y: -80, name: 'ForTask', relevance: 0.94},
    {x: -320, y: 10, name: 'RankSymbols', relevance: 0.87},
    {x: -140, y: -160, name: 'ComputeHITS', relevance: 0.72},
    {x: -380, y: -90, name: 'packBudget', relevance: 0.65},
    {x: -270, y: 100, name: 'seedsForTask', relevance: 0.81},
    {x: -100, y: 30, name: 'RWR', relevance: 0.78},
    {x: -400, y: 50, name: 'TokenBudget', relevance: 0.45},
    {x: -180, y: 160, name: 'ContextBlock', relevance: 0.52},
    {x: -320, y: -170, name: 'HITSScores', relevance: 0.68},
  ];
  c0.forEach(n => nodeData.push({...n, community: 0}));

  // Community 1: MCP server (top-right)
  const c1 = [
    {x: 200, y: -180, name: 'Server', relevance: 0.31},
    {x: 260, y: -140, name: 'handleBlast', relevance: 0.28},
    {x: 150, y: -220, name: 'registerTools', relevance: 0.22},
    {x: 300, y: -200, name: 'requireHash', relevance: 0.15},
    {x: 180, y: -130, name: 'NewServer', relevance: 0.25},
    {x: 250, y: -240, name: 'feedback', relevance: 0.35},
    {x: 320, y: -150, name: 'testScope', relevance: 0.18},
    {x: 140, y: -170, name: 'communities', relevance: 0.12},
  ];
  c1.forEach(n => nodeData.push({...n, community: 1}));

  // Community 2: Indexer (bottom-left)
  const c2 = [
    {x: -150, y: 220, name: 'IndexRepo', relevance: 0.15},
    {x: -220, y: 260, name: 'Extractor', relevance: 0.12},
    {x: -100, y: 280, name: 'GoExtractor', relevance: 0.08},
    {x: -280, y: 240, name: 'extractFile', relevance: 0.10},
    {x: -180, y: 300, name: 'treeSitter', relevance: 0.05},
    {x: -60, y: 240, name: 'FindAll', relevance: 0.07},
    {x: -240, y: 190, name: 'Register', relevance: 0.06},
    {x: -120, y: 180, name: 'Worker', relevance: 0.04},
  ];
  c2.forEach(n => nodeData.push({...n, community: 2}));

  // Community 3: Store (right) — spread for no overlap
  const c3 = [
    {x: 280, y: 40, name: 'SQLiteStore', relevance: 0.42},
    {x: 370, y: -20, name: 'EdgesFrom', relevance: 0.38},
    {x: 220, y: 130, name: 'EdgesTo', relevance: 0.55},
    {x: 360, y: 110, name: 'NodesByName', relevance: 0.33},
    {x: 160, y: 40, name: 'GetNode', relevance: 0.48},
    {x: 420, y: 60, name: 'BatchPut', relevance: 0.10},
    {x: 280, y: 190, name: 'FilePath', relevance: 0.20},
    {x: 330, y: -70, name: 'Migrate', relevance: 0.05},
    {x: 430, y: 150, name: 'Feedback', relevance: 0.30},
  ];
  c3.forEach(n => nodeData.push({...n, community: 3}));

  // Community 4: Wire format (bottom-right)
  const c4 = [
    {x: 120, y: 200, name: 'Encode', relevance: 0.20},
    {x: 180, y: 240, name: 'Decode', relevance: 0.18},
    {x: 80, y: 260, name: 'GCF', relevance: 0.15},
    {x: 160, y: 290, name: 'Payload', relevance: 0.12},
    {x: 60, y: 200, name: 'Session', relevance: 0.10},
    {x: 220, y: 280, name: 'Binary', relevance: 0.08},
  ];
  c4.forEach(n => nodeData.push({...n, community: 4}));

  // Generate edges (within-community and some cross-community)
  const edgePairs: [number, number][] = [];

  // Within-community edges (dense)
  const communityOffsets = [0, 9, 17, 25, 34];
  const communitySizes = [9, 8, 8, 9, 6];

  for (let c = 0; c < 5; c++) {
    const start = communityOffsets[c];
    const size = communitySizes[c];
    for (let i = 0; i < size; i++) {
      // Connect to 2-3 neighbors within community
      for (let j = i + 1; j < Math.min(i + 3, size); j++) {
        edgePairs.push([start + i, start + j]);
      }
    }
  }

  // Cross-community edges (sparse)
  edgePairs.push([0, 17]); // ForTask -> IndexRepo
  edgePairs.push([5, 27]); // RWR -> EdgesTo
  edgePairs.push([0, 25]); // ForTask -> SQLiteStore
  edgePairs.push([1, 29]); // RankSymbols -> GetNode
  edgePairs.push([4, 26]); // seedsForTask -> EdgesFrom
  edgePairs.push([3, 34]); // packBudget -> Encode
  edgePairs.push([9, 0]);  // Server -> ForTask
  edgePairs.push([14, 25]); // NewServer -> SQLiteStore

  // === DRAW THE GRAPH ===

  const graphGroup = createRef<Node>();
  view.add(<Node ref={graphGroup} y={-40} opacity={0} />);

  // Draw edges first (behind nodes)
  const edgeLines = createRefArray<Line>();
  for (const [from, to] of edgePairs) {
    const fromNode = nodeData[from];
    const toNode = nodeData[to];
    graphGroup().add(
      <Line
        ref={edgeLines}
        points={[
          new Vector2(fromNode.x, fromNode.y),
          new Vector2(toNode.x, toNode.y),
        ]}
        stroke={'#252540'}
        lineWidth={1.5}
        opacity={0.7}
      />
    );
  }

  // Draw nodes
  const nodeCircles = createRefArray<Circle>();
  const nodeLabels = createRefArray<Txt>();

  for (const n of nodeData) {
    const baseSize = 22 + n.relevance * 22;
    graphGroup().add(
      <Circle
        ref={nodeCircles}
        x={n.x}
        y={n.y}
        width={baseSize}
        height={baseSize}
        fill={communities[n.community] + 'cc'}
        stroke={communities[n.community]}
        lineWidth={1.5}
        opacity={0.85}
      />
    );
    graphGroup().add(
      <Txt
        ref={nodeLabels}
        text={n.name}
        x={n.x}
        y={n.y + baseSize / 2 + 12}
        fontSize={9}
        fontFamily="'JetBrains Mono', monospace"
        fill={text}
        opacity={0}
      />
    );
  }

  // Subtitle
  const subtitle = createRef<Txt>();
  view.add(
    <Txt
      ref={subtitle}
      text=""
      fontSize={20}
      fontFamily="'JetBrains Mono', monospace"
      fontWeight={300}
      fill={muted}
      y={420}
    />
  );

  // === ANIMATION ===

  // Act 1: The graph appears
  yield* graphGroup().opacity(1, 1.0);
  yield* subtitle().text('A knowledge graph. 40 symbols. 5 communities.', 0.5);
  yield* waitFor(3.0);

  // Show labels
  yield* all(
    ...nodeLabels.map(l => l.opacity(0.8, 0.5)),
  );
  yield* waitFor(2.0);

  // Act 2: A query enters
  const queryBox = createRef<Rect>();
  const queryText = createRef<Txt>();
  view.add(
    <Rect
      ref={queryBox}
      x={0}
      y={370}
      width={500}
      height={40}
      radius={6}
      fill={'#12121c'}
      stroke={accent}
      lineWidth={1.5}
      opacity={0}
    >
      <Txt
        ref={queryText}
        text=""
        fontSize={14}
        fontFamily="'JetBrains Mono', monospace"
        fill={accent}
      />
    </Rect>
  );

  yield* subtitle().text('', 0.3);
  yield* queryBox().opacity(1, 0.4);
  yield* queryBox().y(340, 0.3);

  // Type the query
  const query = 'context_for_task("implement HITS reranking")';
  for (let i = 0; i <= query.length; i++) {
    yield* queryText().text(query.slice(0, i), 0.02);
  }
  yield* waitFor(1.0);

  // Act 3: RWR ripple from seed nodes
  // Seeds: ForTask (0), RankSymbols (1), ComputeHITS (2), HITSScores (8)
  const seedIndices = [0, 1, 2, 8];

  yield* subtitle().text('Seeds identified. Random walk begins.', 0.5);

  // Seeds pulse bright, labels push further below
  yield* all(
    ...seedIndices.map(i => nodeCircles[i].scale(1.6, 0.5)),
    ...seedIndices.map(i => nodeCircles[i].opacity(1.0, 0.4)),
    ...seedIndices.map(i => {
      const n = nodeData[i];
      const baseSize = 22 + n.relevance * 22;
      return nodeLabels[i].y(n.y + (baseSize * 1.6) / 2 + 14, 0.5);
    }),
  );
  yield* waitFor(1.0);

  // Ripple: first ring (high relevance neighbors)
  const ring1 = [3, 4, 5, 6, 7]; // packBudget, seedsForTask, RWR, TokenBudget, ContextBlock
  yield* all(
    ...ring1.map(i => nodeCircles[i].scale(1.3, 0.5)),
    ...ring1.map(i => nodeCircles[i].opacity(1.0, 0.4)),
    ...ring1.map(i => {
      const n = nodeData[i];
      const baseSize = 22 + n.relevance * 22;
      return nodeLabels[i].y(n.y + (baseSize * 1.3) / 2 + 14, 0.5);
    }),
  );
  yield* waitFor(0.8);

  // Ripple: second ring (cross-community, store nodes used by context)
  const ring2 = [25, 27, 29]; // SQLiteStore, EdgesTo, GetNode
  yield* all(
    ...ring2.map(i => nodeCircles[i].scale(1.2, 0.5)),
    ...ring2.map(i => nodeCircles[i].opacity(1.0, 0.4)),
    ...ring2.map(i => {
      const n = nodeData[i];
      const baseSize = 22 + n.relevance * 22;
      return nodeLabels[i].y(n.y + (baseSize * 1.2) / 2 + 14, 0.5);
    }),
  );
  yield* waitFor(1.0);

  // Act 4: Irrelevant nodes fade
  yield* subtitle().text('Irrelevant symbols fade. Relevant ones surface.', 0.5);

  const relevantIndices = new Set([...seedIndices, ...ring1, ...ring2]);
  const irrelevantIndices = nodeData
    .map((_, i) => i)
    .filter(i => !relevantIndices.has(i));

  yield* all(
    ...irrelevantIndices.map(i => nodeCircles[i].opacity(0.08, 1.0)),
    ...irrelevantIndices.map(i => nodeCircles[i].scale(0.4, 1.0)),
    ...irrelevantIndices.map(i => {
      const n = nodeData[i];
      // Push away from center
      const dx = n.x > 0 ? 40 : -40;
      const dy = n.y > 0 ? 30 : -30;
      return nodeCircles[i].position(new Vector2(n.x + dx, n.y + dy - 40), 1.0);
    }),
    ...irrelevantIndices.map(i => nodeLabels[i].opacity(0, 0.6)),
    ...edgeLines.map(e => e.opacity(0.05, 1.0)),
  );
  yield* waitFor(2.0);

  // Act 5: Top-K lift out into ranked list
  yield* subtitle().text('Top-K symbols ranked by score. The context pack.', 0.5);

  // Ranked results panel (right side)
  const resultPanel = createRef<Node>();
  view.add(<Node ref={resultPanel} x={500} opacity={0} />);

  const topK = [
    {name: 'ForTask', score: '0.94', idx: 0},
    {name: 'RankSymbols', score: '0.87', idx: 1},
    {name: 'seedsForTask', score: '0.81', idx: 4},
    {name: 'RWR', score: '0.78', idx: 5},
    {name: 'ComputeHITS', score: '0.72', idx: 2},
    {name: 'HITSScores', score: '0.68', idx: 8},
    {name: 'packBudget', score: '0.65', idx: 3},
    {name: 'EdgesTo', score: '0.55', idx: 27},
  ];

  const resultEntries = createRefArray<Txt>();
  for (let i = 0; i < topK.length; i++) {
    const r = topK[i];
    resultPanel().add(
      <Txt
        ref={resultEntries}
        text={`${(i + 1).toString().padStart(2)}. ${r.name.padEnd(14)} ${r.score}`}
        fontSize={14}
        fontFamily="'JetBrains Mono', monospace"
        fill={text}
        y={-140 + i * 32}
        opacity={0}
      />
    );
  }

  resultPanel().add(
    <Txt
      text="context pack"
      fontSize={11}
      fontFamily="'JetBrains Mono', monospace"
      fill={muted}
      y={-180}
    />
  );

  // Slide graph left and show results
  yield* all(
    graphGroup().x(-150, 0.8),
    resultPanel().opacity(1, 0.6),
    resultPanel().x(380, 0.8),
  );

  // Results appear one by one
  for (let i = 0; i < topK.length; i++) {
    yield* resultEntries[i].opacity(1, 0.15);
    yield* waitFor(0.15);
  }
  yield* waitFor(2.0);

  // Act 6: Token comparison
  yield* subtitle().text('One call. 2,400 tokens. Graph-ranked.', 0.5);
  yield* waitFor(3.0);

  // Fade everything
  yield* all(
    graphGroup().opacity(0, 0.8),
    resultPanel().opacity(0, 0.8),
    queryBox().opacity(0, 0.5),
    subtitle().text('', 0.4),
  );
  yield* waitFor(0.5);

  // Closing tagline
  const tagline = createRef<Txt>();
  view.add(
    <Txt
      ref={tagline}
      text="The graph produces its own context."
      fontSize={30}
      fontFamily="'JetBrains Mono', monospace"
      fontWeight={700}
      fill={text}
      opacity={0}
    />
  );
  yield* tagline().opacity(1, 0.8);
  yield* waitFor(4.0);
});
