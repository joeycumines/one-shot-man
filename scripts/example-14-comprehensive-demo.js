#!/usr/bin/env osm script
// ============================================================================
// example-14-comprehensive-demo.js
// OSM Grand Interactive Dashboard — "The Showcase"
//
// A fully interactive Bubble Tea TUI demonstrating the complete termui
// component ecosystem, compositor, layout engine, viewport, textarea,
// scrollbar, and ClaudeMux multi-agent simulation.
//
// Controls:
//   Tab / Shift+Tab   Switch between views
//   ↑ / ↓             Navigate / scroll
//   ← / →             Adjust / switch sub-views
//   Enter             Activate / confirm
//   Esc               Back / dismiss
//   q / Ctrl+C        Quit
//
// Views:
//   [Overview]    System dashboard with live metrics, sparklines, progress
//   [Showcase]    Interactive gallery of every termui component
//   [Compose]     Visual composition editor: add/move/resize/remove panes
//   [Agents]      ClaudeMux multi-agent simulation with mock provider
//   [Builder]     Dynamic layout builder: split, grid, and stack modes
//   [Log]         Live log viewer with viewport + scrollbar
//   [Editor]      Interactive textarea editor with status bar
//   [Help]        Keyboard shortcut reference
// ============================================================================

try {
    var tea   = require('osm:bubbletea');
    var lip   = require('osm:lipgloss');
    var comp  = require('osm:termui/compositor');
    var coord = require('osm:termui/coordinate');
    var lbl   = require('osm:termui/label');
    var lst   = require('osm:termui/list');
    var lay   = require('osm:termui/layout');
    var bx    = require('osm:termui/box');
    var pnl   = require('osm:termui/panel');
    var div   = require('osm:termui/divider');
    var tbl   = require('osm:termui/table');
    var mdl   = require('osm:termui/modal');
    var tst   = require('osm:termui/toast');
    var sb    = require('osm:termui/scrollbar');
    var sv    = require('osm:termui/splitview');
    var cm    = require('osm:claudemux');
    var vp    = require('osm:bubbles/viewport');
    var ta    = require('osm:bubbles/textarea');

    // ── Theme ──────────────────────────────────────────────
    var dark = true;
    try { dark = lip.hasDarkBackground(); } catch (e) {}

    var C = dark
        ? { bg:'#1A1B26', surface:'#24283b', muted:'#565f89', text:'#c0caf5',
            bright:'#d5d6db', cyan:'#7dcfff', blue:'#7aa2f7',
            purple:'#bb9af7', green:'#9ece6a', yellow:'#e0af68',
            orange:'#ff9e64', red:'#f7768e', border:'#3b4261',
            selBg:'#33467c', focus:'#7aa2f7', chrome:'#292e42',
            dim:'#787c99', barBg:'#1f2335', barFill:'#7aa2f7',
            barOk:'#9ece6a', barWarn:'#e0af68', barErr:'#f7768e' }
        : { bg:'#f5f5f5', surface:'#e8e8e8', muted:'#707070', text:'#1a1a2e',
            bright:'#000000', cyan:'#006080', blue:'#1a3d7a',
            purple:'#5a1fa0', green:'#1a6a2a', yellow:'#8a600b',
            orange:'#b04000', red:'#b00030', border:'#a0a0a0',
            selBg:'#c0d0f0', focus:'#1a3d7a', chrome:'#dcdcdc',
            dim:'#505050', barBg:'#d0d0d0', barFill:'#1a3d7a',
            barOk:'#1a6a2a', barWarn:'#8a600b', barErr:'#b00030' };

    // ── Styles ─────────────────────────────────────────────
    var S = {};
    S.title    = lip.newStyle().bold(true).foreground(C.bright).background(C.chrome).padding(0,1);
    S.hdr      = lip.newStyle().bold(true).foreground(C.purple);
    S.hdr2     = lip.newStyle().bold(true).foreground(C.cyan);
    S.normal   = lip.newStyle().foreground(C.text);
    S.dim      = lip.newStyle().foreground(C.muted);
    S.accent   = lip.newStyle().foreground(C.cyan);
    S.focus    = lip.newStyle().bold(true).foreground(C.focus);
    S.border   = lip.newStyle().foreground(C.border);
    S.sel      = lip.newStyle().background(C.selBg).foreground(C.text).bold(true);
    S.hi       = lip.newStyle().bold(true).foreground(C.yellow);
    S.ok       = lip.newStyle().foreground(C.green);
    S.err      = lip.newStyle().foreground(C.red);
    S.warn     = lip.newStyle().foreground(C.orange);
    S.foot     = lip.newStyle().foreground(C.dim).background(C.chrome).padding(0,1);
    S.tag      = lip.newStyle().background(C.focus).foreground(C.bg).bold(true).padding(0,1);
    S.tagAlt   = lip.newStyle().background(C.purple).foreground(C.bg).bold(true).padding(0,1);
    S.tagOk    = lip.newStyle().background(C.green).foreground(C.bg).bold(true).padding(0,1);
    S.tagWarn  = lip.newStyle().background(C.yellow).foreground(C.bg).bold(true).padding(0,1);
    S.tagErr   = lip.newStyle().background(C.red).foreground(C.bg).bold(true).padding(0,1);
    S.spark    = lip.newStyle().foreground(C.cyan);
    S.sparkHi  = lip.newStyle().foreground(C.yellow);
    S.barBg    = lip.newStyle().background(C.barBg);
    S.barFill  = lip.newStyle().background(C.barFill);
    S.barOk    = lip.newStyle().background(C.barOk);
    S.barWarn  = lip.newStyle().background(C.barWarn);
    S.barErr   = lip.newStyle().background(C.barErr);
    S.key      = lip.newStyle().foreground(C.yellow).bold(true);
    S.code     = lip.newStyle().foreground(C.cyan).background(C.surface).padding(0,1);

    // ── Constants ──────────────────────────────────────────
    var TICK_MS = 250;
    var SPINNER = ['⠋','⠙','⠹','⠸','⠼','⠴','⠦','⠧','⠇','⠏'];
    var SPARK  = ['▁','▂','▃','▄','▅','▆','▇','█'];
    var ROUNDED_BORDER = {top:'─',bottom:'─',left:'│',right:'│',topLeft:'╭',topRight:'╮',bottomLeft:'╰',bottomRight:'╯'};
    var NL = String.fromCharCode(10);

    // ── Helpers ────────────────────────────────────────────
    function r(x,y,w,h) { return {x:x,y:y,width:w,height:h}; }
    function padR(s,n) { s=String(s); while(s.length<n)s+=' '; return s.substring(0,n); }
    function padL(s,n) { s=String(s); while(s.length<n)s=' '+s; return s.substring(s.length-n); }
    function max(a,b) { return a>b?a:b; }
    function min(a,b) { return a<b?a:b; }
    function clamp(v,lo,hi) { return max(lo,min(hi,v)); }
    function join(a,b) { var s=''; for(var i=0;i<a.length;i++){s+=a[i];if(i<a.length-1)s+=b;}return s; }
    function repeat(s,n) { var r=''; for(var i=0;i<n;i++) r+=s; return r; }
    function trunc(s,n) { s=String(s); return s.length>n?s.substring(0,n):s; }

    // Sparkline: array of numbers → string of block chars
    function sparkline(data, lo, hi) {
        if(!data||!data.length) return '';
        if(lo===undefined){lo=data[0];for(var mi=1;mi<data.length;mi++)if(data[mi]<lo)lo=data[mi];}
        if(hi===undefined){hi=data[0];for(var mi=1;mi<data.length;mi++)if(data[mi]>hi)hi=data[mi];}
        var range = hi-lo || 1;
        var s='';
        for(var i=0;i<data.length;i++) {
            var idx = Math.floor(((data[i]-lo)/range)*(SPARK.length-1));
            idx = clamp(idx, 0, SPARK.length-1);
            s += SPARK[idx];
        }
        return s;
    }

    // Progress bar string
    function progressBar(filled, total, w) {
        if(w<1) return '';
        var pct = total>0 ? filled/total : 0;
        var n = Math.round(pct*w);
        var bar = repeat('█',n) + repeat('░',w-n);
        return bar;
    }

    // ── Showcase items ─────────────────────────────────────
    var SHOWCASE_ITEMS = [
        {label:'Box',       demo:'Boxed container with title and border'},
        {label:'Label',     demo:'Styled text: bold, color, alignment'},
        {label:'Divider',   demo:'Horizontal and vertical separator lines'},
        {label:'List',      demo:'Selectable list with highlight'},
        {label:'Panel',     demo:'Bordered panel grouping content'},
        {label:'Table',     demo:'Grid with headers and styled cells'},
        {label:'Modal',     demo:'Centered dialog overlay'},
        {label:'Toast',     demo:'Ephemeral notification'},
        {label:'Scrollbar', demo:'Thin vertical scroll indicator'},
        {label:'Coordinate',demo:'Position, Size, and Rect geometry'},
        {label:'Layout',    demo:'Split, grid, and stack arrangements'},
        {label:'SplitView', demo:'Two-pane horizontal split'},
        {label:'Compositor',demo:'Z-ordered pane compositing'},
        {label:'Viewport',  demo:'Scrollable content viewport'},
        {label:'Textarea',  demo:'Multi-line text editor'},
    ];

    // ── Log entries for the log viewer ─────────────────────
    var LOG_LEVELS = ['DEBUG','INFO','WARN','ERROR'];
    var LOG_COLORS = {DEBUG:C.muted, INFO:C.green, WARN:C.yellow, ERROR:C.red};
    var LOG_MESSAGES = [
        'Initializing OSM runtime engine',
        'Loading native modules: bubbletea, lipgloss, termui',
        'Script discovery: found 14 example scripts',
        'Session manager created with memory backend',
        'Compositor initialized: 80x24 canvas',
        'Theme detection: dark background detected',
        'All termui components registered successfully',
        'Starting interactive dashboard...',
        'Rendering frame 1: 13 showcase items loaded',
        'User navigation: Showcase → Compose tab switch',
        'Pane editor: 2 panes active, mode=nav',
        'Agent simulation: Planner agent initialized',
        'Agent simulation: Coder agent initialized',
        'Agent simulation: Reviewer agent initialized',
        'Layout builder: split mode, horizontal direction',
        'Viewport: 45 lines of content, scroll at 0%',
        'Textarea: editor ready, 0 characters',
        'Modal overlay: opened, awaiting user input',
        'Toast notification: "Welcome to OSM Dashboard"',
        'Scrollbar: content=45, viewport=12, offset=0',
        'Table rendered: 5 rows x 3 columns',
        'List rendered: 15 items, selected index 0',
        'Panel: "System Metrics" rendered with border',
        'Box: "Quick Stats" container rendered',
        'Divider: horizontal line at y=5',
        'Label: "CPU Usage: 42%" styled text',
        'Coordinate: rect(0,0,80,24) created',
        'SplitView: 60/40 horizontal split rendered',
        'Compositor: 3 panes composited successfully',
        'Tick: frame updated, spinner advanced',
        'Memory: heap=24MB, gc=3 cycles',
        'Network: 0 requests (offline mode)',
        'Clipboard: output ready for paste',
        'Session: state persisted to memory store',
        'Dashboard: all views rendered without errors',
    ];

    // ── State ──────────────────────────────────────────────
    var state = {
        tick:0, view:0, menuIdx:0,
        modalOpen:false, modalMsg:'',
        toastOpen:false, toastMsg:'', toastTime:0,
        // Compose
        composePanes:[
            {id:'main',label:'Main',x:0,y:0,w:36,h:8},
            {id:'side',label:'Side',x:37,y:0,w:36,h:8},
        ],
        composeSelected:0, composeMode:'nav',
        // Agents
        agents:[
            {role:'Planner',state:'idle',tasks:0},
            {role:'Coder',state:'idle',tasks:0},
            {role:'Reviewer',state:'idle',tasks:0},
            {role:'Tester',state:'idle',tasks:0},
        ],
        agentSelected:0, agentSimRunning:false,
        agentLogs:[[],[],[],[]],
        // Builder
        builderMode:'split', builderDir:'horizontal', builderSelected:0,
        builderPanes:[{label:'A'},{label:'B'}],
        // Overview metrics
        cpuHistory:[], memHistory:[], netHistory:[],
        cpu:0, mem:0, net:0,
        // Log viewer
        logScroll:0, logFilter:0, // 0=all, 1=DEBUG, 2=INFO, 3=WARN, 4=ERROR
        // Editor
        editorText:'# OSM Dashboard Editor\n#\n# This is a live textarea component.\n# Type to edit, Esc to clear.\n#\n# Try the keyboard shortcuts:\n#   Ctrl+K — clear all\n#   ↑↓ — navigate lines\n#   Type anything!\n',
        editorFocused:false,
        // Help scroll
        helpScroll:0,
    };

    // Seed initial metric history
    for(var i=0;i<40;i++) {
        state.cpuHistory.push(20+Math.random()*30);
        state.memHistory.push(40+Math.random()*20);
        state.netHistory.push(Math.random()*10);
    }

    // ── Simulation step ────────────────────────────────────
    function simStep() {
        var a = state.agents[state.agentSelected];
        if(a.state==='idle') { a.state='working'; a.tasks++; }
        else if(a.state==='working') { a.state='done'; }
        else { a.state='idle'; }
        var msgs = [
            'Analyzing task requirements...',
            'Generating implementation plan...',
            'Writing code for module...',
            'Running tests...',
            'Reviewing output quality...',
            'Task complete. Moving to next.',
            'Idle. Awaiting new instructions.',
        ];
        var log = msgs[state.tick % msgs.length];
        state.agentLogs[state.agentSelected].push(log);
        if(state.agentLogs[state.agentSelected].length > 50)
            state.agentLogs[state.agentSelected].shift();
    }

    // ── Update metrics ─────────────────────────────────────
    function updateMetrics() {
        state.cpu = clamp(state.cpu + (Math.random()-0.5)*10, 5, 95);
        state.mem = clamp(state.mem + (Math.random()-0.5)*5, 20, 90);
        state.net = clamp(state.net + (Math.random()-0.5)*4, 0, 15);
        state.cpuHistory.push(state.cpu);
        state.memHistory.push(state.mem);
        state.netHistory.push(state.net);
        if(state.cpuHistory.length > 40) state.cpuHistory.shift();
        if(state.memHistory.length > 40) state.memHistory.shift();
        if(state.netHistory.length > 40) state.netHistory.shift();
    }

    // ── Showcase detail ────────────────────────────────────
    function showcaseDetail(idx) {
        var demos = [
            // Box
            function() {
                var b = bx.box({title:'Box Demo',style:lip.newStyle().foreground(C.border),border:ROUNDED_BORDER});
                return b.render(r(0,0,30,5));
            },
            // Label
            function() {
                var lines = [];
                lines.push(lbl.label('Bold Label',{style:S.hi}).render(r(0,0,30,1)));
                lines.push(lbl.label('Cyan Label',{style:S.accent}).render(r(0,0,30,1)));
                lines.push(lbl.label('Dim Label',{style:S.dim}).render(r(0,0,30,1)));
                lines.push(lbl.label('Error Label',{style:S.err}).render(r(0,0,30,1)));
                return join(lines,NL);
            },
            // Divider
            function() {
                var lines = [];
                lines.push(S.dim.render('Horizontal:'));
                lines.push(div.divider('horizontal',{style:S.border}).render(r(0,0,28,1)));
                lines.push('');
                lines.push(S.dim.render('With custom char:'));
                lines.push(div.divider('horizontal',{style:S.accent,char:'═'}).render(r(0,0,28,1)));
                return join(lines,NL);
            },
            // List
            function() {
                var items = [{text:'Alpha'},{text:'Beta'},{text:'Gamma'},{text:'Delta'}];
                var l = lst.list({items:items,selectedStyle:S.sel,selectedIndex:state.tick%4});
                return l.render(r(0,0,28,6));
            },
            // Panel
            function() {
                var p = pnl.panel({title:'Panel Demo',style:lip.newStyle().foreground(C.border),border:ROUNDED_BORDER});
                return p.render(r(0,0,30,5));
            },
            // Table
            function() {
                var t = tbl.table({headers:['Name','Status','Load'],rows:[['alpha','ok','12%'],['beta','warn','67%'],['gamma','ok','34%']],headerStyle:S.hi,cellStyle:S.normal});
                return t.render(r(0,0,30,6));
            },
            // Modal
            function() { return S.dim.render('Press Enter in Showcase to open modal'); },
            // Toast
            function() { return S.dim.render('Press Enter in Showcase to trigger toast'); },
            // Scrollbar
            function() {
                var s = sb.new(6);
                s.setContentHeight(20);
                s.setYOffset(Math.floor(state.tick%3));
                s.setThumbBackground(C.focus);
                s.setTrackForeground(C.border);
                return s.view();
            },
            // Coordinate
            function() {
                var lines = [];
                var p = coord.position({x:5,y:3});
                var sz = coord.size({width:80,height:24});
                var rc = coord.rect({x:0,y:0,width:80,height:24});
                lines.push(S.accent.render('Position:')+' '+p.toString());
                lines.push(S.accent.render('Size:')+' '+sz.toString()+' area='+sz.area());
                lines.push(S.accent.render('Rect:')+' '+rc.toString());
                lines.push(S.dim.render('  contains(5,3)='+p.in(rc)));
                var halves = rc.split(0.5,true);
                lines.push(S.dim.render('  split(0.5) → ['+halves[0].toString()+', ...]'));
                return join(lines,NL);
            },
            // Layout
            function() {
                var rc = coord.rect({x:0,y:0,width:28,height:6});
                var cells = lay.grid(rc,3,2);
                var lines = [];
                lines.push(S.accent.render('Grid 3×2:'));
                for(var i=0;i<cells.length;i++) {
                    lines.push('  ['+i+'] '+cells[i].toString());
                }
                return join(lines,NL);
            },
            // SplitView
            function() {
                var left = lbl.label('Left Pane',{style:S.accent});
                var right = lbl.label('Right Pane',{style:S.dim});
                var s = sv.splitView({primary:left,secondary:right,ratio:0.5});
                return s.render(r(0,0,30,3));
            },
            // Compositor
            function() {
                var c = comp.compositor({width:28,height:5});
                c.addPane({id:'a',content:S.hi.render('Pane A'),bounds:{x:0,y:0,width:14,height:5},z:0});
                c.addPane({id:'b',content:S.accent.render('Pane B'),bounds:{x:14,y:0,width:14,height:5},z:1});
                return c.render();
            },
            // Viewport
            function() {
                var lines = [];
                lines.push(S.accent.render('Scrollable Viewport:'));
                lines.push(S.dim.render('  45 lines of content'));
                lines.push(S.dim.render('  Scroll with ↑↓ in Log view'));
                lines.push(S.dim.render('  Integrated with scrollbar'));
                return join(lines,NL);
            },
            // Textarea
            function() {
                var lines = [];
                lines.push(S.accent.render('Multi-line Text Editor:'));
                lines.push(S.dim.render('  Go to [Editor] tab to try'));
                lines.push(S.dim.render('  Supports Ctrl+K to clear'));
                lines.push(S.dim.render('  Live character count'));
                return join(lines,NL);
            },
        ];
        var fn = demos[idx];
        if(!fn) return 'Select a module.';
        try { return fn(); } catch(e) { return S.err.render('Error: '+e.message); }
    }

    // ── Render: Overview (system dashboard) ────────────────
    function renderOverview(m, W, H) {
        var cH = H - 4;
        var lines = [];
        lines.push(S.hdr.render('  System Overview'));
        lines.push(div.divider('horizontal',{style:S.border}).render(r(0,0,W,1)));
        lines.push('');
        lines.push('  CPU: ' + Math.round(m.cpu) + '%  MEM: ' + Math.round(m.mem) + '%  NET: ' + m.net.toFixed(1) + ' MB/s');
        lines.push('');
        var cpuSpark = sparkline(m.cpuHistory);
        lines.push(S.accent.render('  CPU ') + S.spark.render(cpuSpark));
        var memSpark = sparkline(m.memHistory);
        lines.push(S.accent.render('  MEM ') + S.spark.render(memSpark));
        var netSpark = sparkline(m.netHistory);
        lines.push(S.accent.render('  NET ') + S.spark.render(netSpark));
        lines.push('');
        var procTable = tbl.table({headers:['PID','Process','CPU%','MEM%','Status'],
            rows:[
                ['1234','osm-engine','12.3','45.2','running'],
                ['5678','script-runtime','8.7','23.1','running'],
                ['9012','compositor','3.4','12.8','idle'],
                ['3456','agent-sim','1.2','8.4','working'],
                ['7890','log-viewer','0.5','4.1','idle'],
            ],
            headerStyle:S.hi,cellStyle:S.normal
        });
        lines.push(S.hdr2.render('  Processes:'));
        lines.push(procTable.render(r(2,0,W-4,max(3,cH-lines.length-2))));
        return join(lines,NL);
    }

    // ── Render: Showcase ───────────────────────────────────
    function renderShowcase(m, W, H) {
        var cH = H - 4;
        var listW = 26;
        var items = [];
        for(var li=0;li<SHOWCASE_ITEMS.length;li++) {
            items.push({text:(li===m.menuIdx?'► ':'  ')+SHOWCASE_ITEMS[li].label});
        }
        var lc = lst.list({items:items,selectedStyle:S.sel,selectedIndex:m.menuIdx});
        var listStr = lc.render(r(0,0,listW,cH));
        var detailTxt = showcaseDetail(m.menuIdx);
        var ll=listStr.split(NL);
        var dl=detailTxt.split(NL);
        var ml=max(ll.length,dl.length);
        var lines = [];
        for(var cl=0;cl<ml;cl++) {
            var lv=cl<ll.length?ll[cl]:'';
            var dv=cl<dl.length?dl[cl]:'';
            lines.push(padR(lv,listW)+' │ '+dv);
        }
        lines.push('');
        lines.push(S.dim.render('  ↑↓:nav  Enter:demo  Tab:switch view'));
        return join(lines,NL);
    }

    // ── Render: Compose ────────────────────────────────────
    function renderCompose(m, W, H) {
        var cH = H - 4;
        var lines = [];
        lines.push(S.hdr.render('  Composition Editor'));
        lines.push(div.divider('horizontal',{style:S.border}).render(r(0,0,W,1)));
        lines.push('');
        lines.push(S.dim.render('  Panes: '+m.composePanes.length+'  Mode: '+m.composeMode+'  Selected: '+m.composeSelected));
        lines.push('');
        for(var pi=0;pi<m.composePanes.length;pi++) {
            var p=m.composePanes[pi];
            var txt='  '+(pi===m.composeSelected?'► ':'  ')+p.label+' @('+p.x+','+p.y+') '+p.w+'×'+p.h;
            lines.push(pi===m.composeSelected?S.sel.render(txt):S.normal.render(txt));
        }
        lines.push('');
        // Visual composition using compositor
        var compW = min(70, W-4);
        var compH = max(3, cH - lines.length - 2);
        var c = comp.compositor({width:compW,height:compH});
        for(var ci=0;ci<m.composePanes.length;ci++) {
            var cp=m.composePanes[ci];
            var paneContent = '';
            var b = bx.box({title:cp.label,border:ROUNDED_BORDER,style:ci===m.composeSelected?S.focus:S.border});
            paneContent = b.render(r(0,0,max(5,cp.w),max(3,cp.h)));
            c.addPane({id:cp.id,content:paneContent,bounds:{x:cp.x,y:cp.y,width:cp.w,height:cp.h},z:ci});
        }
        var compRendered = c.render();
        var compLines = compRendered.split(NL);
        for(var cli=0;cli<compLines.length && lines.length<cH;cli++) {
            lines.push('  '+compLines[cli]);
        }
        lines.push('');
        lines.push(S.dim.render('  m=move  r=resize  n=new  d=del  tab=cycle  arrows=adjust'));
        return join(lines,NL);
    }

    // ── Render: Agents ─────────────────────────────────────
    function renderAgents(m, W, H) {
        var cH = H - 4;
        var lines = [];
        lines.push(S.hdr.render('  ClaudeMux Multi-Agent Simulation'));
        lines.push(div.divider('horizontal',{style:S.border}).render(r(0,0,W,1)));
        lines.push('');
        var simTag = m.agentSimRunning ? S.tagOk.render(' RUNNING ') : S.tag.render(' PAUSED ');
        lines.push('  Status: '+simTag+'  Agents: '+m.agents.length+'  Selected: '+m.agentSelected);
        lines.push('');
        for(var ai=0;ai<m.agents.length;ai++) {
            var a=m.agents[ai];
            var si=a.state==='idle'?'○':a.state==='working'?SPINNER[m.tick%SPINNER.length]:'●';
            var stateStyle=a.state==='idle'?S.dim:a.state==='working'?S.warn:S.ok;
            var line='  '+si+' '+padR(a.role,12)+' │ '+stateStyle.render(padR(a.state,10))+' │ tasks:'+a.tasks;
            lines.push(ai===m.agentSelected?S.sel.render(line):S.normal.render(line));
        }
        lines.push('');
        // Agent detail panel
        var sa=m.agents[m.agentSelected];
        if(sa) {
            var detailLines = [];
            detailLines.push(S.hdr2.render('  Agent: '+sa.role));
            detailLines.push(S.dim.render('  State: '+sa.state+'  Tasks completed: '+sa.tasks));
            detailLines.push('');
            if(m.agentLogs[m.agentSelected] && m.agentLogs[m.agentSelected].length) {
                detailLines.push(S.accent.render('  Recent logs:'));
                var logs=m.agentLogs[m.agentSelected];
                for(var lj=max(0,logs.length-5);lj<logs.length;lj++) {
                    detailLines.push(S.dim.render('    '+SPINNER[m.tick%SPINNER.length]+' '+logs[lj]));
                }
            } else {
                detailLines.push(S.dim.render('    No logs yet. Press Enter to step.'));
            }
            var detailStr = join(detailLines,NL);
            var dl = detailStr.split(NL);
            for(var di=0;di<dl.length && lines.length<cH-2;di++) {
                lines.push(dl[di]);
            }
        }
        lines.push('');
        lines.push(S.dim.render('  ↑↓:nav  Enter:step  s:auto  x:stop  Tab:switch'));
        return join(lines,NL);
    }

    // ── Render: Builder ────────────────────────────────────
    function renderBuilder(m, W, H) {
        var cH = H - 4;
        var lines = [];
        lines.push(S.hdr.render('  Dynamic Layout Builder'));
        lines.push(div.divider('horizontal',{style:S.border}).render(r(0,0,W,1)));
        lines.push('');
        lines.push(S.dim.render('  Mode: '+m.builderMode+'  Dir: '+m.builderDir+'  Panes: '+m.builderPanes.length+'  Selected: '+m.builderSelected));
        lines.push('');

        // Show pane list
        for(var bi=0;bi<m.builderPanes.length;bi++) {
            var txt='  '+(bi===m.builderSelected?'► ':'  ')+m.builderPanes[bi].label;
            lines.push(bi===m.builderSelected?S.sel.render(txt):S.normal.render(txt));
        }
        lines.push('');

        // Visual layout preview using splitview
        var previewH = max(4, cH - lines.length - 3);
        var paneCount = m.builderPanes.length;
        if(paneCount >= 2) {
            var dir = m.builderDir === 'horizontal' ? sv.Direction.HORIZONTAL : sv.Direction.VERTICAL;
            // Build a nested splitview
            var primary = lbl.label('  '+m.builderPanes[0].label,{style:S.accent});
            var secondary = lbl.label('  '+m.builderPanes[1].label,{style:S.dim});
            var s = sv.splitView({primary:primary,secondary:secondary,ratio:0.5,direction:dir});
            var rendered = s.render(r(2,0,W-4,previewH));
            var rl = rendered.split(NL);
            for(var ri=0;ri<rl.length;ri++) lines.push(rl[ri]);
        }
        lines.push('');
        lines.push(S.dim.render('  s/g/t:mode  v:h:dir  +/-:panes  ↑↓:select  Tab:switch'));
        return join(lines,NL);
    }

    // ── Render: Log viewer ─────────────────────────────────
    function renderLog(m, W, H) {
        var cH = H - 4;
        var lines = [];
        lines.push(S.hdr.render('  Live Log Viewer'));
        lines.push(div.divider('horizontal',{style:S.border}).render(r(0,0,W,1)));
        lines.push('');

        // Filter indicator
        var filterLabel = m.logFilter===0 ? S.tag.render(' ALL ') :
            S.tagAlt.render(' '+LOG_LEVELS[m.logFilter-1]+' ');
        lines.push('  Filter: '+filterLabel+'  Lines: '+LOG_MESSAGES.length+'  Scroll: '+m.logScroll);
        lines.push('');

        // Build filtered log content
        var logLines = [];
        for(var li=0;li<LOG_MESSAGES.length;li++) {
            var lvl = LOG_LEVELS[li % LOG_LEVELS.length];
            var lvlIdx = LOG_LEVELS.indexOf(lvl);
            if(m.logFilter > 0 && lvlIdx !== m.logFilter-1) continue;
            var ts = new Date().toISOString().substring(11,19);
            var color = LOG_COLORS[lvl] || C.text;
            var style = lip.newStyle().foreground(color);
            logLines.push(lbl.label(ts+' ['+padR(lvl,5)+'] '+LOG_MESSAGES[li],{style:style}).render(r(0,0,W-4,1)));
        }

        // Show visible window
        var visibleCount = cH - 6;
        var startLine = clamp(m.logScroll, 0, max(0, logLines.length-visibleCount));
        for(var si=0;si<visibleCount && (startLine+si)<logLines.length;si++) {
            lines.push('  '+logLines[startLine+si]);
        }

        // Scrollbar
        var scrollbarH = cH;
        var s = sb.new(scrollbarH);
        s.setContentHeight(logLines.length);
        s.setViewportHeight(visibleCount);
        s.setYOffset(startLine);
        s.setThumbBackground(C.focus);
        s.setTrackForeground(C.border);
        var sbView = s.view();
        var sbLines = sbView.split(NL);

        // Merge scrollbar into right edge
        var mergedLines = [];
        for(var mi=0;mi<lines.length;mi++) {
            var sbIdx = mi - 2; // offset for header lines
            var sbChar = (sbIdx >= 0 && sbIdx < sbLines.length) ? sbLines[sbIdx] : '';
            mergedLines.push(padR(lines[mi],W-2)+sbChar);
        }

        mergedLines.push('');
        mergedLines.push(S.dim.render('  ↑↓:scroll  0-4:filter  Tab:switch'));
        return join(mergedLines,NL);
    }

    // ── Render: Editor ─────────────────────────────────────
    function renderEditor(m, W, H) {
        var cH = H - 4;
        var lines = [];
        lines.push(S.hdr.render('  Text Editor'));
        lines.push(div.divider('horizontal',{style:S.border}).render(r(0,0,W,1)));
        lines.push('');

        // Textarea
        var taH = max(4, cH - 6);
        var taW = W - 4;
        var editor = ta.new();
        editor.setWidth(taW);
        editor.setHeight(taH);
        editor.setValue(m.editorText);
        if(m.editorFocused) editor.focus(); else editor.blur();

        var taView = editor.view();
        var taLines = taView.split(NL);
        for(var ti=0;ti<taLines.length;ti++) {
            lines.push('  '+taLines[ti]);
        }

        lines.push('');
        var charCount = m.editorText.length;
        var lineCount = m.editorText.split(NL).length;
        var focusTag = m.editorFocused ? S.tagOk.render(' FOCUSED ') : S.tag.render(' BLURRED ');
        lines.push('  '+focusTag+'  Chars: '+charCount+'  Lines: '+lineCount);
        lines.push(S.dim.render('  f:focus  Esc:blur  Ctrl+K:clear  Tab:switch'));
        return join(lines,NL);
    }

    // ── Render: Help ───────────────────────────────────────
    function renderHelp(m, W, H) {
        var cH = H - 4;
        var lines = [];
        lines.push(S.hdr.render('  Keyboard Shortcuts'));
        lines.push(div.divider('horizontal',{style:S.border}).render(r(0,0,W,1)));
        lines.push('');

        var shortcuts = [
            ['Tab','Switch to next view'],
            ['Shift+Tab','Switch to previous view'],
            ['↑ / ↓','Navigate / scroll'],
            ['← / ↓','Adjust / switch sub-views'],
            ['Enter','Activate / confirm'],
            ['Esc','Back / dismiss / blur editor'],
            ['q / Ctrl+C','Quit application'],
            ['',''],
            ['Overview','Live system metrics dashboard'],
            ['Showcase','Browse all termui components'],
            ['Compose','Visual pane composition editor'],
            ['Agents','Multi-agent simulation'],
            ['Builder','Dynamic layout builder'],
            ['Log','Live log viewer with filter'],
            ['Editor','Interactive text editor'],
            ['Help','This reference page'],
            ['',''],
            ['Compose: m','Enter move mode'],
            ['Compose: r','Enter resize mode'],
            ['Compose: n','Add new pane'],
            ['Compose: d','Delete selected pane'],
            ['',''],
            ['Agents: s','Start auto-step'],
            ['Agents: x','Stop auto-step'],
            ['',''],
            ['Builder: s/g/t','Split/grid/stack mode'],
            ['Builder: v/h','Vertical/horizontal direction'],
            ['Builder: +/-','Add/remove panes'],
            ['',''],
            ['Log: 0-4','Filter by log level'],
            ['',''],
            ['Editor: f','Focus editor'],
            ['Editor: Ctrl+K','Clear all text'],
        ];

        for(var si=0;si<shortcuts.length;si++) {
            var key = shortcuts[si][0];
            var desc = shortcuts[si][1];
            if(!key) { lines.push(''); continue; }
            lines.push('  '+S.key.render(padR(key,18))+'  '+S.dim.render(desc));
        }

        return join(lines,NL);
    }

    // ── Main render dispatcher ─────────────────────────────
    function renderView(m, W, H) {
        var tabs = ['Overview','Showcase','Compose','Agents','Builder','Log','Editor','Help'];
        var hdrH = 2, ftrH = 1, divH = 1, cH = H - hdrH - ftrH - divH;

        // Header
        var themeLabel = dark ? ' DARK ' : ' LIGHT ';
        var titleRaw = 'OSM Grand Interactive Dashboard'+themeLabel;
        var tabRaw = '';
        for(var i=0;i<tabs.length;i++) {
            tabRaw += (i===m.view ? S.tag.render(' '+tabs[i]+' ') : ' '+tabs[i]+' ');
            if(i<tabs.length-1) tabRaw += S.border.render('│');
        }
        var fullTitle = padR(titleRaw+'  '+tabRaw, W);
        var hdr = S.title.render(fullTitle);

        // Divider
        var divider = div.divider('horizontal',{style:S.border}).render(r(0,0,W,1));

        // Footer
        var help = '';
        if(m.view===0) help='Tab:switch  ↑↓:scroll  q:quit';
        else if(m.view===1) help='Tab:switch  ↑↓:nav  Enter:demo  q:quit';
        else if(m.view===2) help='Tab:switch  ↑↓:nav  m:move  r:resize  n:new  d:del  q:quit';
        else if(m.view===3) help='Tab:switch  ↑↓:nav  Enter:step  s:auto  x:stop  q:quit';
        else if(m.view===4) help='Tab:switch  s/g/t:mode  v/h:dir  +/-  q:quit';
        else if(m.view===5) help='Tab:switch  ↑↓:scroll  0-4:filter  q:quit';
        else if(m.view===6) help='Tab:switch  f:focus  Esc:blur  Ctrl+K:clear  q:quit';
        else help='Tab:switch  q:quit';
        var spin = SPINNER[m.tick % SPINNER.length];
        var ftr = S.foot.render(padR(spin+' View: '+tabs[m.view]+' │ '+help, W));

        // Content
        var content = '';
        if(m.view===0) content = renderOverview(m, W, cH);
        else if(m.view===1) content = renderShowcase(m, W, cH);
        else if(m.view===2) content = renderCompose(m, W, cH);
        else if(m.view===3) content = renderAgents(m, W, cH);
        else if(m.view===4) content = renderBuilder(m, W, cH);
        else if(m.view===5) content = renderLog(m, W, cH);
        else if(m.view===6) content = renderEditor(m, W, cH);
        else content = renderHelp(m, W, cH);

        return hdr + NL + divider + NL + content + NL + ftr;
    }

    // ── Program ────────────────────────────────────────────
    var program = tea.newModel({
        init: function() {
            return [state, tea.tick(TICK_MS, 'tick')];
        },
        update: function(msg, s) {
            if(!msg || !msg.type) return [s, null];
            if(msg.type==='Tick') {
                s.tick++;
                updateMetrics();
                if(s.agentSimRunning) simStep();
                if(s.toastOpen && s.tick - s.toastTime > 6) s.toastOpen = false;
                return [s, tea.tick(TICK_MS, 'tick')];
            }
            if(msg.type==='WindowSize') return [s,null];
            if(msg.type==='Key') {
                if(msg.key==='q'||msg.key==='ctrl+c') return [s,tea.quit()];
                if(s.modalOpen) {
                    if(msg.key==='enter'||msg.key==='q'||msg.key==='escape') s.modalOpen=false;
                    return [s,null];
                }
                if(msg.key==='tab'){s.view=(s.view+1)%8;s.menuIdx=0;s.agentSelected=0;s.builderSelected=0;return[s,null];}
                if(msg.key==='shift+tab'){s.view=(s.view+7)%8;s.menuIdx=0;s.agentSelected=0;s.builderSelected=0;return[s,null];}

                if(s.view===0){ // Overview
                    // no-op for now, metrics auto-update
                } else if(s.view===1){ // Showcase
                    if(msg.key==='down'){s.menuIdx=min(s.menuIdx+1,SHOWCASE_ITEMS.length-1);return[s,null];}
                    if(msg.key==='up'){s.menuIdx=max(s.menuIdx-1,0);return[s,null];}
                    if(msg.key==='enter'){
                        if(SHOWCASE_ITEMS[s.menuIdx].label==='Modal'){s.modalOpen=true;s.modalMsg='Modal dialog.\n\nThis is a real modal overlay.\n\nPress Enter or q to dismiss.';}
                        else{s.toastOpen=true;s.toastMsg='✓ '+SHOWCASE_ITEMS[s.menuIdx].label+' demo activated';s.toastTime=s.tick;}
                        return[s,null];
                    }
                } else if(s.view===2){ // Compose
                    if(msg.key==='m'){s.composeMode='move';return[s,null];}
                    if(msg.key==='r'){s.composeMode='resize';return[s,null];}
                    if(msg.key==='n'){s.composePanes.push({id:'p'+(s.composePanes.length+1),label:'P'+(s.composePanes.length+1),x:Math.floor(Math.random()*40),y:Math.floor(Math.random()*6),w:20,h:5});s.composeSelected=s.composePanes.length-1;return[s,null];}
                    if(msg.key==='d'){if(s.composePanes.length>1){s.composePanes.splice(s.composeSelected,1);s.composeSelected=min(s.composeSelected,s.composePanes.length-1);}return[s,null];}
                    if(msg.key==='tab'){s.composeSelected=(s.composeSelected+1)%s.composePanes.length;return[s,null];}
                    if(msg.key==='shift+tab'){s.composeSelected=(s.composeSelected-1+s.composePanes.length)%s.composePanes.length;return[s,null];}
                    var p=s.composePanes[s.composeSelected];
                    if(s.composeMode==='move'){
                        if(msg.key==='down'){p.y=min(p.y+1,15);return[s,null];}
                        if(msg.key==='up'){p.y=max(p.y-1,0);return[s,null];}
                        if(msg.key==='right'){p.x=min(p.x+1,60);return[s,null];}
                        if(msg.key==='left'){p.x=max(p.x-1,0);return[s,null];}
                    } else if(s.composeMode==='resize'){
                        if(msg.key==='right'){p.w=min(p.w+1,80-p.x);return[s,null];}
                        if(msg.key==='left'){p.w=max(p.w-1,5);return[s,null];}
                        if(msg.key==='down'){p.h=min(p.h+1,20-p.y);return[s,null];}
                        if(msg.key==='up'){p.h=max(p.h-1,3);return[s,null];}
                    } else {
                        if(msg.key==='down'){s.composeSelected=min(s.composeSelected+1,s.composePanes.length-1);return[s,null];}
                        if(msg.key==='up'){s.composeSelected=max(s.composeSelected-1,0);return[s,null];}
                    }
                } else if(s.view===3){ // Agents
                    if(msg.key==='down'){s.agentSelected=min(s.agentSelected+1,s.agents.length-1);return[s,null];}
                    if(msg.key==='up'){s.agentSelected=max(s.agentSelected-1,0);return[s,null];}
                    if(msg.key==='enter'){simStep();return[s,null];}
                    if(msg.key==='s'){s.agentSimRunning=true;return[s,null];}
                    if(msg.key==='x'){s.agentSimRunning=false;return[s,null];}
                } else if(s.view===4){ // Builder
                    if(msg.key==='s'){s.builderMode='split';return[s,null];}
                    if(msg.key==='g'){s.builderMode='grid';return[s,null];}
                    if(msg.key==='t'){s.builderMode='stack';return[s,null];}
                    if(msg.key==='v'){s.builderDir='vertical';return[s,null];}
                    if(msg.key==='h'){s.builderDir='horizontal';return[s,null];}
                    if(msg.key==='+'){s.builderPanes.push({label:String.fromCharCode(65+s.builderPanes.length)});return[s,null];}
                    if(msg.key==='-'){if(s.builderPanes.length>2)s.builderPanes.pop();return[s,null];}
                    if(msg.key==='down'){s.builderSelected=min(s.builderSelected+1,s.builderPanes.length-1);return[s,null];}
                    if(msg.key==='up'){s.builderSelected=max(s.builderSelected-1,0);return[s,null];}
                } else if(s.view===5){ // Log
                    if(msg.key==='down'){s.logScroll=min(s.logScroll+1,50);return[s,null];}
                    if(msg.key==='up'){s.logScroll=max(s.logScroll-1,0);return[s,null];}
                    if(msg.key==='0'){s.logFilter=0;return[s,null];}
                    if(msg.key==='1'){s.logFilter=1;return[s,null];}
                    if(msg.key==='2'){s.logFilter=2;return[s,null];}
                    if(msg.key==='3'){s.logFilter=3;return[s,null];}
                    if(msg.key==='4'){s.logFilter=4;return[s,null];}
                } else if(s.view===6){ // Editor
                    if(msg.key==='f'){s.editorFocused=true;return[s,null];}
                    if(msg.key==='escape'){s.editorFocused=false;return[s,null];}
                    if(msg.key==='ctrl+k'){s.editorText='';return[s,null];}
                    // Pass printable chars to textarea
                    if(msg.text && msg.text.length===1 && msg.key.length===1) {
                        s.editorText += msg.text;
                        return[s,null];
                    }
                    if(msg.key==='enter'){s.editorText+=NL;return[s,null];}
                    if(msg.key==='backspace'||msg.key==='backspace2'){
                        if(s.editorText.length>0) s.editorText=s.editorText.substring(0,s.editorText.length-1);
                        return[s,null];
                    }
                } else { // Help
                    if(msg.key==='down'){s.helpScroll++;return[s,null];}
                    if(msg.key==='up'){s.helpScroll=max(s.helpScroll-1,0);return[s,null];}
                }
            }
            return [s,null];
        },
        view: function(model) {
            var content = renderView(model, 80, 24);
            // Modal overlay
            if(model.modalOpen) {
                var modalContent = lbl.label(model.modalMsg,{style:S.normal});
                var md = mdl.modal({content:modalContent,width:40,height:10,style:lip.newStyle().background(C.surface).foreground(C.text),border:ROUNDED_BORDER});
                var modalStr = md.render(r(0,0,80,24));
                // Composite modal over content using compositor
                var c = comp.compositor({width:80,height:24});
                c.addPane({id:'bg',content:content,bounds:{x:0,y:0,width:80,height:24},z:0});
                c.addPane({id:'modal',content:modalStr,bounds:{x:0,y:0,width:80,height:24},z:10});
                content = c.render();
            }
            // Toast overlay
            if(model.toastOpen) {
                var ts = tst.toast({message:model.toastMsg,style:S.ok,width:40});
                var toastStr = ts.render(r(0,0,80,24));
                var c = comp.compositor({width:80,height:24});
                c.addPane({id:'bg',content:content,bounds:{x:0,y:0,width:80,height:24},z:0});
                c.addPane({id:'toast',content:toastStr,bounds:{x:0,y:0,width:80,height:24},z:10});
                content = c.render();
            }
            return {
                content: content,
                altScreen:true,
                mouseMode:'cellMotion',
                reportFocus:true,
            };
        },
    });

    tea.run(program);
} catch(e) {
    console.error('FATAL: '+e.message);
    if(e.stack) console.error(e.stack);
    throw e;
}
