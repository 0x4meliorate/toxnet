package main

const indexHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Toxnet</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body { margin: 0; height: 100vh; display: flex; flex-direction: column; overflow: hidden;
    font: 14px/1.5 system-ui, sans-serif; background: #15171c; color: #e6e8eb; }
  header { padding: 10px 16px; background: #1c1f26; border-bottom: 1px solid #2a2e37;
    display: flex; gap: 12px; align-items: center; flex: 0 0 auto; }
  header b { font-weight: 600; }
  .mono { font-family: ui-monospace, "Cascadia Code", Consolas, monospace; }
  #id { font-size: 12px; color: #9aa4b2; background: #23272f; padding: 4px 8px; border-radius: 6px;
    cursor: pointer; max-width: 46ch; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  #dls { margin-left: auto; flex: 0 0 auto; display: flex; gap: 8px; }
  .dl { display: inline-flex; align-items: center; gap: 6px; padding: 6px 12px; border-radius: 6px;
    background: #2a2f38; border: 1px solid #353a44; color: #e6e8eb; text-decoration: none; font-size: 13px; }
  .dl:hover { background: #333945; }
  .dl svg { width: 15px; height: 15px; }
  .oswrap { width: 18px; flex: 0 0 auto; display: inline-flex; align-items: center; justify-content: center; }
  .oswrap svg { width: 17px; height: 17px; }
  input, button { font: inherit; padding: 7px 10px; border-radius: 6px; border: 1px solid #353a44;
    background: #23272f; color: #e6e8eb; }
  button { background: #2d6cdf; border-color: #2d6cdf; cursor: pointer; }
  button:hover { background: #3b78e7; }
  button:disabled { opacity: .4; cursor: default; }


  #friends { flex: 1; overflow-y: auto; min-height: 0; }
  .fr { display: flex; align-items: center; gap: 14px; padding: 11px 16px;
    border-bottom: 1px solid #22262e; cursor: pointer; }
  .fr:hover { background: #1c1f26; }
  .fr.openrow { box-shadow: inset 3px 0 #2d6cdf; }
  .dot { width: 9px; height: 9px; border-radius: 50%; background: #555; flex: 0 0 auto; }
  .dot.on { background: #41c766; }
  .fid { font-size: 12px; color: #7f8896; width: 108px; flex: 0 0 auto; }
  .nick { width: 160px; flex: 0 0 auto; font-size: 13px; padding: 6px 10px; }
  .seen { font-size: 12px; color: #7f8896; width: 170px; flex: 0 0 auto; }
  .note { flex: 1; min-width: 0; font-size: 12px; padding: 6px 10px; }

  #friendbar { flex: 0 0 auto; display: flex; gap: 8px; align-items: center;
    padding: 8px 16px; background: #1a1d23; border-bottom: 1px solid #2a2e37; }
  #frtabs { display: flex; gap: 2px; flex: 0 0 auto; }
  .frtab { background: #23272f; border: 1px solid #353a44; color: #9aa4b2; padding: 6px 12px; font-size: 13px; }
  .frtab:hover { background: #2a2f38; color: #e6e8eb; }
  .frtab.active { background: #2d6cdf; border-color: #2d6cdf; color: #fff; }
  #frsearch { flex: 1; min-width: 60px; font-size: 13px; padding: 6px 10px; }
  #frsort { padding: 6px 10px; font-size: 13px; background: #23272f; border: 1px solid #353a44;
    color: #e6e8eb; border-radius: 6px; cursor: pointer; flex: 0 0 auto; }
  #frdir { background: #2a2f38; border: 1px solid #353a44; color: #e6e8eb; padding: 6px 11px; flex: 0 0 auto; }
  #frdir:hover { background: #333945; }
  #frempty { padding: 18px 16px; color: #7f8896; font-size: 13px; }
  body.popped > #friendbar { display: none !important; }
  #addbtn { flex: 0 0 auto; }

  .unread { width: 8px; height: 8px; border-radius: 50%; background: #4d9fff; flex: 0 0 auto;
    box-shadow: 0 0 6px #4d9fff; }
  .utab { width: 7px; height: 7px; border-radius: 50%; background: #4d9fff; box-shadow: 0 0 5px #4d9fff;
    flex: 0 0 auto; }
  .fid { cursor: default; }
  #fidtip { position: fixed; display: none; z-index: 30; background: #23272f; border: 1px solid #353a44;
    border-radius: 6px; padding: 8px 10px; box-shadow: 0 6px 20px rgba(0,0,0,.4); cursor: pointer; max-width: 92vw; }
  #fidtip.open { display: block; }
  #fidtip:hover { border-color: #4d7fd0; }
  #fidtip .fidval { font-size: 12px; color: #e6e8eb; word-break: break-all; max-width: 560px; }
  #fidtip .fidhint { font-size: 11px; color: #7f8896; margin-top: 5px; }

  #dock { flex: 0 0 auto; display: none; flex-direction: column; background: #15171c;
    border-top: 1px solid #2a2e37; min-height: 0; }
  #handle { height: 11px; flex: 0 0 auto; cursor: ns-resize; background: #1c1f26;
    border-bottom: 1px solid #2a2e37; position: relative; }
  #handle::after { content: ""; position: absolute; left: 50%; top: 50%; transform: translate(-50%, -50%);
    width: 44px; height: 3px; border-radius: 2px; background: #3a4150; }
  #handle:hover::after { background: #6fb3ff; }
  #tabs { flex: 0 0 auto; display: flex; gap: 2px; padding: 0 8px; overflow-x: auto; border-bottom: 1px solid #2a2e37; }
  .tab { display: inline-flex; align-items: center; gap: 7px; flex: 0 0 auto; padding: 9px 12px;
    cursor: pointer; font-size: 13px; color: #9aa4b2; border-bottom: 2px solid transparent; }
  .tab:hover { color: #e6e8eb; }
  .tab.active { color: #e6e8eb; border-bottom-color: #2d6cdf; }
  .tab .x { opacity: .5; padding: 0 2px; }
  .tab .x:hover { opacity: 1; color: #ff6b6b; }
  .tab svg { width: 14px; height: 14px; }
  .tab { cursor: grab; }
  .tab.dragging { opacity: .4; }
  .tab.drop-before { box-shadow: inset 2px 0 0 #2d6cdf; }
  .tab.drop-after { box-shadow: inset -2px 0 0 #2d6cdf; }
  .tab .pop { opacity: .45; padding: 0 2px; font-size: 12px; }
  .tab .pop:hover { opacity: 1; color: #6fb3ff; }
  /* Popped-out window: ?pop=<kind><num> shows one tab full-window, no chrome. */
  body.popped > header, body.popped > #friends, body.popped #tabs, body.popped #handle { display: none !important; }
  body.popped #dock { position: fixed !important; inset: 0 !important; height: auto !important; display: flex !important; }
  #pophdr { display: none; }
  body.popped #pophdr { display: flex; align-items: center; gap: 7px; flex: 0 0 auto;
    position: sticky; top: 0; z-index: 2; padding: 11px 16px; font-size: 14px; color: #e6e8eb;
    background: #15171c; border-bottom: 1px solid #2a2e37; }
  #pophdr svg { width: 15px; height: 15px; }
  #cwrap { flex: 1; display: flex; flex-direction: column; min-height: 0; }
  #console { flex: 1; overflow-y: auto; padding: 12px 16px; font-size: 13px;
    white-space: pre-wrap; word-break: break-word; }
  #console .line { padding: 1px 0; }
  #console .out { color: #6fb3ff; }
  #console .out::before { content: "> "; color: #4b74a8; }
  #console .in { color: #cdd3dc; }
  #compose { flex: 0 0 auto; display: flex; gap: 8px; padding: 10px; border-top: 1px solid #2a2e37; }
  #compose input { flex: 1; }
  #empty { margin: auto; color: #7f8896; }

  /* file browser */
  #browser { flex: 1; display: none; flex-direction: column; min-height: 0; }
  #fbbar { flex: 0 0 auto; display: flex; gap: 6px; align-items: center; padding: 8px 12px;
    border-bottom: 1px solid #2a2e37; }
  #fbbar button { padding: 5px 10px; }
  #fbpath { flex: 1; min-width: 0; font-size: 12px; color: #cdd3dc; background: #1c1f26;
    border: 1px solid #2a2e37; border-radius: 6px; padding: 6px 10px; overflow: hidden;
    text-overflow: ellipsis; white-space: nowrap; }
  #fblist { flex: 1; overflow-y: auto; padding: 6px 0; }
  .ent { display: flex; align-items: center; gap: 10px; padding: 5px 16px; cursor: default; font-size: 13px; }
  .ent:hover { background: #1c1f26; }
  .ent.dir { cursor: pointer; }
  .ent svg { width: 16px; height: 16px; flex: 0 0 auto; }
  .ent .nm { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ent .prog { width: 120px; height: 7px; background: #2a2f38; border-radius: 4px; overflow: hidden; flex: 0 0 auto; }
  .ent .prog .bar { display: block; height: 100%; background: #2d6cdf; }
  .ent .pct { font-size: 11px; color: #9aa4b2; width: 58px; text-align: right; flex: 0 0 auto; }
  .ent .save { font-size: 12px; color: #41c766; text-decoration: none; flex: 0 0 auto; }
  .ent .save:hover { text-decoration: underline; }
  .ent .dlerr { font-size: 12px; color: #ff6b6b; flex: 0 0 auto; }
  .fbmsg { color: #7f8896; padding: 16px; }

  .menu { position: fixed; display: none; background: #23272f; border: 1px solid #353a44;
    border-radius: 6px; padding: 4px; box-shadow: 0 6px 20px rgba(0,0,0,.4); z-index: 10; }
  .menu { padding: 0; }
  .menu > div { padding: 9px 16px; cursor: pointer; white-space: nowrap; position: relative; }
  .menu > div:first-child { border-radius: 6px 6px 0 0; }
  .menu > div:last-child { border-radius: 0 0 6px 6px; }
  .menu > div:only-child { border-radius: 6px; }
  .menu > div:not(:last-child) { border-bottom: 1px solid #2f343d; }
  .menu > div:hover { background: #243041; }
  #rm:hover { background: #3a2730; color: #ff6b6b; }
  #submenu { display: none; position: absolute; left: 100%; top: -1px; margin-left: 3px;
    background: #23272f; border: 1px solid #353a44; border-radius: 6px; padding: 0; overflow: hidden; }
  /* Transparent bridge over the gap so moving onto the submenu keeps it open. */
  #submenu::before { content: ""; position: absolute; left: -7px; top: 0; width: 7px; height: 100%; }
  #submenu > div { padding: 12px 24px; font-size: 15px; cursor: pointer; }
  #submenu > div:not(:last-child) { border-bottom: 1px solid #2f343d; }
  #submenu > div:hover { background: #243041; }
  #submenu.open { display: block; }
  .caret { float: right; opacity: .6; margin-left: 18px; }
  .dcaret { opacity: .55; font-size: 11px; }
  #dlmenu > div { display: flex; align-items: center; gap: 7px; }
  #dlmenu svg { width: 15px; height: 15px; }

  /* download controls + store */
  .dlbtn { padding: 3px 9px; font-size: 12px; background: #2a2f38; border: 1px solid #353a44; color: #e6e8eb; }
  .dlbtn:hover { background: #333945; }
  .dlbtn.cancel { color: #ff9a9a; }
  #dlbtn, #addbtn, #logsbtn { background: #2a2f38; border-color: #353a44; display: inline-flex; align-items: center; gap: 6px; }
  #dlbtn:hover, #addbtn:hover, #logsbtn:hover { background: #333945; }
  #dlbadge { background: #2d6cdf; color: #fff; border-radius: 10px; font-size: 11px; padding: 0 7px; display: none; }
  #store { position: fixed; inset: 0; background: rgba(9,11,15,.72); z-index: 20;
    display: none; align-items: flex-start; justify-content: center; padding: 46px 16px; }
  #store.open { display: flex; }
  #storebox { background: #1b1e25; border: 1px solid #2a2e37; border-radius: 10px; width: 880px;
    max-width: 100%; max-height: 100%; display: flex; flex-direction: column; overflow: hidden; box-shadow: 0 16px 50px rgba(0,0,0,.5); }
  #storehd { display: flex; align-items: center; gap: 10px; padding: 13px 18px; border-bottom: 1px solid #2a2e37; }
  #storehd b { font-size: 15px; }
  #storehd .close { margin-left: auto; cursor: pointer; color: #9aa4b2; font-size: 22px; line-height: 1; padding: 0 4px; }
  #storehd .close:hover { color: #e6e8eb; }
  #storelist { overflow-y: auto; }
  .drow { display: flex; align-items: center; gap: 14px; padding: 11px 18px; border-bottom: 1px solid #22262e; }
  .drow:last-child { border-bottom: none; }
  .drow .dmeta { flex: 1 1 auto; min-width: 0; }
  .drow .dname { font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .drow .dsub { font-size: 11px; color: #7f8896; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; margin-top: 2px; }
  .drow .dsub .frm { color: #6fb3ff; }
  .drow .dright { flex: 0 0 auto; display: flex; align-items: center; gap: 8px; }
  .drow .dsize { font-size: 12px; color: #9aa4b2; min-width: 62px; text-align: right; }
  .drow .prog { width: 110px; }
  .storeempty { color: #7f8896; padding: 28px; text-align: center; }

  /* add-friend dialog */
  #addmodal { position: fixed; inset: 0; background: rgba(9,11,15,.72); z-index: 20;
    display: none; align-items: flex-start; justify-content: center; padding: 90px 16px; }
  #addmodal.open { display: flex; }
  #addbox { background: #1b1e25; border: 1px solid #2a2e37; border-radius: 10px; width: 540px;
    max-width: 100%; box-shadow: 0 16px 50px rgba(0,0,0,.5); }
  #addhd { display: flex; align-items: center; gap: 10px; padding: 13px 18px; border-bottom: 1px solid #2a2e37; }
  #addhd b { font-size: 15px; }
  #addhd .close { margin-left: auto; cursor: pointer; color: #9aa4b2; font-size: 22px; line-height: 1; padding: 0 4px; }
  #addhd .close:hover { color: #e6e8eb; }
  #addform { display: flex; flex-direction: column; gap: 10px; padding: 16px 18px; }
  #addform input { width: 100%; }
  #addform button { align-self: flex-start; }
  #adderr { color: #ff8f8f; font-size: 12px; }
  #adderr:empty { display: none; }

  /* activity log */
  #logsmodal { position: fixed; inset: 0; background: rgba(9,11,15,.72); z-index: 20;
    display: none; align-items: flex-start; justify-content: center; padding: 40px 16px; }
  #logsmodal.open { display: flex; }
  #logsbox { background: #1b1e25; border: 1px solid #2a2e37; border-radius: 10px; width: 1040px;
    max-width: 100%; max-height: 100%; display: flex; flex-direction: column; overflow: hidden; box-shadow: 0 16px 50px rgba(0,0,0,.5); }
  #logshd { display: flex; align-items: center; gap: 14px; padding: 10px 18px; border-bottom: 1px solid #2a2e37; }
  #logshd b { font-size: 15px; flex: 0 0 auto; }
  #logstabs { display: flex; gap: 2px; flex: 1 1 auto; }
  .ltab { padding: 7px 12px; cursor: pointer; font-size: 13px; color: #9aa4b2; border-bottom: 2px solid transparent; }
  .ltab:hover { color: #e6e8eb; }
  .ltab.active { color: #e6e8eb; border-bottom-color: #2d6cdf; }
  #logshd .close { cursor: pointer; color: #9aa4b2; font-size: 22px; line-height: 1; padding: 0 4px; flex: 0 0 auto; }
  #logshd .close:hover { color: #e6e8eb; }
  #logsbody { overflow: auto; }
  table.logt { width: 100%; border-collapse: collapse; font-size: 12px; }
  table.logt th { text-align: left; padding: 8px 12px; color: #9aa4b2; border-bottom: 1px solid #2a2e37;
    position: sticky; top: 0; background: #1b1e25; white-space: nowrap; }
  table.logt td { padding: 7px 12px; border-bottom: 1px solid #22262e; vertical-align: top; white-space: nowrap; }
  table.logt td.wrap { white-space: pre-wrap; word-break: break-word; max-width: 560px;
    font-family: ui-monospace, Consolas, monospace; }
  table.logt tr.clickrow { cursor: pointer; }
  table.logt tr.clickrow:hover td { background: #22262e; }
  .logempty { color: #7f8896; padding: 24px; text-align: center; }
  #logsfoot { display: flex; align-items: center; gap: 12px; padding: 8px 18px; border-top: 1px solid #2a2e37; flex: 0 0 auto; }
  #logspage { font-size: 12px; color: #9aa4b2; }

  /* full console text */
  #textmodal { position: fixed; inset: 0; background: rgba(9,11,15,.72); z-index: 21;
    display: none; align-items: flex-start; justify-content: center; padding: 60px 16px; }
  #textmodal.open { display: flex; }
  #textbox { background: #1b1e25; border: 1px solid #2a2e37; border-radius: 10px; width: 780px;
    max-width: 100%; max-height: 100%; display: flex; flex-direction: column; overflow: hidden; box-shadow: 0 16px 50px rgba(0,0,0,.5); }
  #texthead { display: flex; align-items: center; gap: 10px; padding: 11px 18px; border-bottom: 1px solid #2a2e37; }
  #texthd { font-size: 13px; color: #cdd3dc; flex: 1 1 auto; font-weight: 400; }
  #texthead .close { cursor: pointer; color: #9aa4b2; font-size: 22px; line-height: 1; }
  #texthead .close:hover { color: #e6e8eb; }
  #textbody { margin: 0; overflow: auto; padding: 14px 18px; white-space: pre-wrap; word-break: break-word;
    font: 12px/1.5 ui-monospace, Consolas, monospace; color: #e6e8eb; }
</style>
</head>
<body>
<header>
  <b>Toxnet</b>
  <span>Tox ID:</span>
  <span id="id" class="mono" title="click to copy">...</span>
  <span id="dls">
    <button id="dlbtn" class="dl" type="button" title="Loot">Loot<span id="dlbadge"></span></button>
    <button id="logsbtn" class="dl" type="button" title="Activity log">Logs</button>
    <button id="dldrop" class="dl" type="button" title="Download a remote shell">Download<span class="dcaret">&#9662;</span></button>
  </span>
</header>
<div id="friendbar">
  <span id="frtabs">
    <button class="frtab active" data-cat="shells" type="button">Shells</button>
    <button class="frtab" data-cat="people" type="button">People</button>
  </span>
  <input id="frsearch" class="mono" placeholder="Search friends…" autocomplete="off">
  <select id="frsort" title="Sort by">
    <option value="seen">Last online</option>
    <option value="os">OS</option>
    <option value="name">Name</option>
  </select>
  <button id="frdir" type="button" title="Toggle sort direction">&#9660;</button>
  <button id="addbtn" class="dl" type="button" title="Add a friend by Tox ID">Add Friend</button>
</div>
<div id="friends"></div>
<div id="dock">
  <div id="handle" title="drag to resize"></div>
  <div id="tabs"></div>
  <div id="cwrap">
    <div id="pophdr"></div>
    <div id="console"></div>
    <div id="browser">
      <div id="fbbar">
        <button id="fbback" title="Back">&#8592;</button>
        <button id="fbfwd" title="Forward">&#8594;</button>
        <button id="fbup" title="Up">&#8593;</button>
        <span id="fbpath" class="mono"></span>
      </div>
      <div id="fblist"></div>
    </div>
    <form id="compose">
      <input id="text" class="mono" placeholder="Type a command or message" autocomplete="off">
      <button type="submit">Send</button>
    </form>
  </div>
</div>
<div id="store">
  <div id="storebox">
    <div id="storehd"><b>Loot</b><span class="close" id="storeclose" title="Close">&times;</span></div>
    <div id="storelist"></div>
  </div>
</div>
<div id="logsmodal">
  <div id="logsbox">
    <div id="logshd">
      <b>Activity Log</b>
      <span id="logstabs">
        <span class="ltab active" data-t="downloads">Loot</span>
        <span class="ltab" data-t="console">Console</span>
        <span class="ltab" data-t="friends">Friends</span>
        <span class="ltab" data-t="connections">Connections</span>
      </span>
      <span class="close" id="logsclose" title="Close">&times;</span>
    </div>
    <div id="logsbody"></div>
    <div id="logsfoot">
      <button id="logsprev" class="dlbtn" type="button">Prev</button>
      <span id="logspage" class="mono"></span>
      <button id="logsnext" class="dlbtn" type="button">Next</button>
    </div>
  </div>
</div>
<div id="textmodal">
  <div id="textbox">
    <div id="texthead"><b id="texthd"></b><span class="close" id="textclose" title="Close">&times;</span></div>
    <pre id="textbody"></pre>
  </div>
</div>
<div id="addmodal">
  <div id="addbox">
    <div id="addhd"><b>Add Friend</b><span class="close" id="addclose" title="Close">&times;</span></div>
    <form id="addform">
      <input id="toxid" class="mono" placeholder="Tox ID (76 hex characters)" autocomplete="off" spellcheck="false">
      <div id="adderr"></div>
      <button type="submit">Add</button>
    </form>
  </div>
</div>
<div id="menu" class="menu">
  <div id="mi-interact">Interact<span class="caret">&#9656;</span>
    <div id="submenu"><div id="mi-console">Console</div><div id="mi-files">File Browser</div></div>
  </div>
  <div id="mi-chat">Open Chat</div>
  <div id="rm">Remove friend</div>
</div>
<div id="fbmenu" class="menu"><div id="mi-dl">Download</div></div>
<div id="dlmenu" class="menu">
  <div data-href="/download/toxnet-windows.exe" data-name="toxnet-windows.exe"></div>
  <div data-href="/download/toxnet-windows.bin" data-name="toxnet-windows.bin"></div>
  <div data-href="/download/toxnet-linux" data-name="toxnet-linux"></div>
</div>
<div id="fidtip"><div class="fidval mono"></div><div class="fidhint">click to copy</div></div>
<script>
let friends = [], tabs = [], active = null, msgs = [], cursor = 0, menuNum = null, dlPath = null;
let popped = {};   // tabKey -> { win, num, kind } for tabs currently open in their own window
let friendCat = 'shells', friendQuery = '', friendSort = 'seen', friendDir = -1;   // -1 desc, 1 asc
const unread = new Set();   // friend nums with incoming messages not yet viewed
let popNotify = null;       // set by a pop-out window to react to incoming messages
const rowEls = new Map();
const fb = new Map(); // num -> {path, os, entries, history, hi, loading, error}
let dockH = 300;
try { const v = parseInt(localStorage.getItem('dockH')); if (v) dockH = v; } catch (e) {}

const WIN_SVG = '<svg viewBox="0 0 24 24"><path fill="#00A4EF" d="M3 4.6l8.2-1.13v8.1H3zM12.3 3.34L21 2.1v9.47h-8.7zM3 12.57h8.2v8.08L3 19.5zM12.3 12.57H21v9.33l-8.7-1.2z"/></svg>';
const TUX_SVG = '<svg viewBox="0 0 24 24">' +
  '<ellipse cx="12" cy="14" rx="6.2" ry="7.6" fill="#1b1b1b"/><ellipse cx="12" cy="16" rx="4" ry="5.6" fill="#f7f7f7"/>' +
  '<ellipse cx="12" cy="7" rx="4.3" ry="4.6" fill="#1b1b1b"/><ellipse cx="10.2" cy="6.6" rx="1.1" ry="1.4" fill="#fff"/>' +
  '<ellipse cx="13.8" cy="6.6" rx="1.1" ry="1.4" fill="#fff"/><circle cx="10.4" cy="6.9" r=".55" fill="#1b1b1b"/>' +
  '<circle cx="13.6" cy="6.9" r=".55" fill="#1b1b1b"/><path d="M10.6 8.4 L12 9.7 L13.4 8.4 Z" fill="#f6a623"/>' +
  '<path d="M8.6 20.5 l1.6 1.8 2.6-.2 -1.2-2z" fill="#f6a623"/><path d="M15.4 20.5 l-1.6 1.8 -2.6-.2 1.2-2z" fill="#f6a623"/></svg>';
const FOLDER_SVG = '<svg viewBox="0 0 24 24"><path fill="#e3b341" d="M3 6a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>';
const FILE_SVG = '<svg viewBox="0 0 24 24"><path fill="#9aa4b2" d="M6 2h8l4 4v16H6z"/><path fill="#cdd3dc" d="M14 2l4 4h-4z"/></svg>';
const PERSON_SVG = '<svg viewBox="0 0 24 24"><path fill="#9aa4b2" d="M12 12c2.21 0 4-1.79 4-4s-1.79-4-4-4-4 1.79-4 4 1.79 4 4 4zm0 2c-2.67 0-8 1.34-8 4v2h16v-2c0-2.66-5.33-4-8-4z"/></svg>';
const UNKNOWN_SVG = '<svg viewBox="0 0 24 24"><path fill="#9aa4b2" d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 17h-2v-2h2v2zm2.07-7.75l-.9.92C13.45 12.9 13 13.5 13 15h-2v-.5c0-1.1.45-2.1 1.17-2.83l1.24-1.26c.37-.36.59-.86.59-1.41 0-1.1-.9-2-2-2s-2 .9-2 2H8c0-2.21 1.79-4 4-4s4 1.79 4 4c0 .88-.36 1.68-.93 2.25z"/></svg>';
function osSVG(f) {
  const n = (f && f.name || '').toLowerCase();
  if (n.includes('linux')) return TUX_SVG;
  if (n.includes('windows')) return WIN_SVG;
  // No OS in the name: a friend we added is a person; one we didn't (a connected
  // shell on an unrecognized OS) gets the unknown-OS icon.
  return (f && f.addedByUs) ? PERSON_SVG : UNKNOWN_SVG;
}
// Download dropdown: one button opens a menu of the remote-shell builds.
const dlMenu = document.getElementById('dlmenu');
const dlLabels = [WIN_SVG + 'Windows (.exe)', WIN_SVG + 'Windows (.bin)', TUX_SVG + 'Linux'];
dlMenu.querySelectorAll('div').forEach((it, i) => {
  it.innerHTML = dlLabels[i];
  it.onclick = () => {
    const a = document.createElement('a');
    a.href = it.dataset.href; a.download = it.dataset.name;
    document.body.appendChild(a); a.click(); a.remove();
  };
});
document.getElementById('dldrop').onclick = e => {
  e.stopPropagation();
  const isOpen = dlMenu.style.display === 'block';
  hideMenus();
  if (isOpen) return;
  const r = e.currentTarget.getBoundingClientRect();
  dlMenu.style.display = 'block';
  dlMenu.style.top = (r.bottom + 4) + 'px';
  dlMenu.style.left = Math.max(8, r.right - dlMenu.offsetWidth) + 'px';
};

async function api(path, opts) {
  const r = await fetch(path, opts);
  if (!r.ok) throw new Error((await r.text()).trim() || r.statusText);
  return r.status === 200 ? r.json() : null;
}
function esc(s) { return s.replace(/[&<>]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;'}[c])); }
function label(num) { const f = friends.find(x => x.num === num); return f ? (f.localName || f.publicKey.slice(0, 12)) : ('#' + num); }
function isOnline(num) { const f = friends.find(x => x.num === num); return f && f.online; }
function ago(sec) {
  if (!sec) return 'never connected';
  const d = Date.now() / 1000 - sec;
  if (d < 60) return 'last seen just now';
  if (d < 3600) return 'last seen ' + Math.floor(d / 60) + 'm ago';
  if (d < 86400) return 'last seen ' + Math.floor(d / 3600) + 'h ago';
  return 'last seen ' + new Date(sec * 1000).toLocaleString();
}

async function loadSelf() {
  const s = await api('/api/self');
  document.getElementById('id').textContent = s.address;
}
document.getElementById('id').onclick = e => navigator.clipboard.writeText(e.target.textContent);

function makeRow(num) {
  const el = document.createElement('div');
  el.className = 'fr';
  el.innerHTML = '<span class="dot"></span><span class="unread"></span><span class="oswrap"></span>' +
    '<input class="nick mono" placeholder="nickname"><span class="fid mono"></span>' +
    '<span class="seen"></span><input class="note mono" placeholder="Add a note…">';
  el.oncontextmenu = e => { e.preventDefault(); showMenu(e.clientX, e.clientY, num); };
  editable(el, 'nick', num, 'name', 'name');
  editable(el, 'note', num, 'note', 'note');
  const fid = el.querySelector('.fid');
  fid.onmouseenter = () => fidTipShow(fid);
  fid.onmouseleave = fidTipHide;
  return el;
}

// Full-ID tooltip: hover a truncated .fid to reveal the full public key; click to copy.
const fidtip = document.getElementById('fidtip');
let fidHideTimer = null, fidCurrent = '';
function fidTipShow(fid) {
  clearTimeout(fidHideTimer);
  fidCurrent = fid.dataset.full || '';
  fidtip.querySelector('.fidval').textContent = fidCurrent;
  fidtip.querySelector('.fidhint').textContent = 'click to copy';
  fidtip.classList.add('open');
  const r = fid.getBoundingClientRect();
  fidtip.style.top = (r.bottom + 4) + 'px';
  fidtip.style.left = Math.max(8, Math.min(r.left, window.innerWidth - fidtip.offsetWidth - 8)) + 'px';
}
function fidTipHide() {
  fidHideTimer = setTimeout(() => fidtip.classList.remove('open'), 220);
}
fidtip.onmouseenter = () => clearTimeout(fidHideTimer);
fidtip.onmouseleave = fidTipHide;
fidtip.onclick = async () => {
  try { await navigator.clipboard.writeText(fidCurrent); fidtip.querySelector('.fidhint').textContent = 'copied!'; }
  catch (e) { fidtip.querySelector('.fidhint').textContent = 'copy failed'; }
};
function editable(el, cls, num, path, key) {
  const inp = el.querySelector('.' + cls);
  inp.onclick = e => e.stopPropagation();
  inp.onkeydown = e => { if (e.key === 'Enter') { e.preventDefault(); inp.blur(); } };
  inp.onchange = async () => {
    try { await api('/api/friends/' + num + '/' + path, { method: 'POST', body: JSON.stringify({ [key]: inp.value }) }); loadFriends(); }
    catch (err) { alert(err.message); }
  };
}
function updateRow(el, f) {
  el.classList.toggle('openrow', tabs.some(t => t.num === f.num));
  el.querySelector('.dot').className = 'dot' + (f.online ? ' on' : '');
  el.querySelector('.oswrap').innerHTML = osSVG(f);
  const fid = el.querySelector('.fid');
  fid.textContent = f.publicKey.slice(0, 12);
  fid.dataset.full = f.publicKey;
  el.querySelector('.unread').style.display = unread.has(f.num) ? '' : 'none';
  el.querySelector('.seen').textContent = f.online ? 'online' : ago(f.lastSeen);
  const nick = el.querySelector('.nick');
  if (document.activeElement !== nick) nick.value = f.localName || '';
  const note = el.querySelector('.note');
  if (document.activeElement !== note) note.value = f.note || '';
}
function osRank(f) { const n = (f.name || '').toLowerCase(); return n.includes('windows') ? 0 : n.includes('linux') ? 1 : 2; }
function friendSortKey(f) {
  if (friendSort === 'os') return osRank(f);
  if (friendSort === 'name') return (f.localName || f.publicKey || '').toLowerCase();
  return f.online ? Infinity : (f.lastSeen || 0);   // 'seen' = last online
}
function friendCmp(a, b) {
  const ka = friendSortKey(a), kb = friendSortKey(b);
  let r = ka < kb ? -1 : ka > kb ? 1 : 0;
  if (r === 0) r = (a.localName || '').localeCompare(b.localName || '') || (a.num - b.num);
  return r * friendDir;
}
// Filter friends by the active tab + search box, sort them, and lay out the rows.
function renderFriendList() {
  const list = document.getElementById('friends');
  // People have no meaningful OS, so drop the OS sort option on that tab.
  const allowOs = friendCat !== 'people';
  const osOpt = document.querySelector('#frsort option[value="os"]');
  osOpt.hidden = !allowOs;
  osOpt.disabled = !allowOs;
  if (!allowOs && friendSort === 'os') { friendSort = 'seen'; document.getElementById('frsort').value = 'seen'; }
  const q = friendQuery.trim().toLowerCase();
  const matches = f => !q || (f.localName || '').toLowerCase().includes(q)
    || (f.publicKey || '').toLowerCase().includes(q) || (f.name || '').toLowerCase().includes(q);
  const inCat = f => friendCat === 'people' ? !!f.addedByUs : !f.addedByUs;
  const shown = friends.filter(f => inCat(f) && matches(f)).sort(friendCmp);
  const shownNums = new Set(shown.map(f => f.num));
  for (const [num, el] of rowEls) if (!shownNums.has(num)) el.style.display = 'none';
  for (const f of shown) {
    let el = rowEls.get(f.num);
    if (!el) { el = makeRow(f.num); rowEls.set(f.num, el); }
    el.style.display = '';
    updateRow(el, f);
    // Re-appending moves the node, which blurs a focused input inside it and
    // interrupts typing. Leave the row being edited in place; it re-sorts on blur.
    if (!el.contains(document.activeElement)) list.appendChild(el);   // re-append -> sorted order
  }
  const people = friends.filter(f => f.addedByUs).length;
  document.querySelector('.frtab[data-cat="shells"]').textContent = 'Shells (' + (friends.length - people) + ')';
  document.querySelector('.frtab[data-cat="people"]').textContent = 'People (' + people + ')';
  document.getElementById('addbtn').style.display = friendCat === 'people' ? '' : 'none';
  let empty = document.getElementById('frempty');
  if (!shown.length) {
    if (!empty) { empty = document.createElement('div'); empty.id = 'frempty'; list.appendChild(empty); }
    empty.textContent = q ? 'No matches.' : 'No friends here yet.';
    empty.style.display = 'block';
  } else if (empty) empty.style.display = 'none';
}
async function loadFriends() {
  friends = await api('/api/friends');
  const nums = new Set(friends.map(f => f.num));
  for (const [num, el] of rowEls) if (!nums.has(num)) { el.remove(); rowEls.delete(num); }
  renderFriendList();
  renderTabs();
}

function tabKey(t) { return t.kind + t.num; }
function updateDock() {
  const dock = document.getElementById('dock');
  if (!tabs.length) { dock.style.display = 'none'; return; }
  dock.style.display = 'flex';
  dock.style.height = Math.max(120, Math.min(window.innerHeight - 160, dockH)) + 'px';
}
function openTab(num, kind) {
  const win = popped[kind + num];   // already open in its own window? focus it, no duplicate tab
  if (win) { try { win.win.focus(); } catch (e) {} return; }
  if (!tabs.some(t => t.num === num && t.kind === kind)) tabs.push({ num, kind });
  active = kind + num;
  if (kind === 'console') unread.delete(num);
  if (kind === 'files' && !fb.get(num)) fbLoad(num, '', true);
  loadFriends();
  updateDock();
  render();
  if (kind === 'console') document.getElementById('text').focus();
}
function closeTab(num, kind) {
  tabs = tabs.filter(t => !(t.num === num && t.kind === kind));
  if (active === kind + num) active = tabs.length ? tabKey(tabs[tabs.length - 1]) : null;
  loadFriends();
  updateDock();
  render();
}
// Open this tab in its own window (a chrome-less single-tab view of the same
// backend) and detach it from the dock. The window is tracked so closing it
// returns the tab, and it can't be reopened as a tab while the window is open.
function popOut(t) {
  const key = tabKey(t);
  const w = window.open(location.pathname + '?pop=' + t.kind + t.num, 'pop_' + key, 'width=760,height=560');
  if (!w) return;   // popup blocked - leave the tab in the dock
  popped[key] = { win: w, num: t.num, kind: t.kind };
  closeTab(t.num, t.kind);
}
let dragTab = null;
// Move the dragged tab to just before or after the drop-target tab, then re-render.
function moveTab(from, to, after) {
  const fi = tabs.indexOf(from);
  if (fi < 0) return;
  tabs.splice(fi, 1);
  const ti = tabs.indexOf(to);
  if (ti < 0) tabs.push(from);
  else tabs.splice(ti + (after ? 1 : 0), 0, from);
  renderTabs();
}
function renderTabs() {
  const el = document.getElementById('tabs');
  el.innerHTML = '';
  for (const t of tabs) {
    const f = friends.find(x => x.num === t.num);
    const d = document.createElement('div');
    d.className = 'tab mono' + (active === tabKey(t) ? ' active' : '');
    const icon = t.kind === 'files' ? FOLDER_SVG
      : '<span class="dot' + (isOnline(t.num) ? ' on' : '') + '"></span>' + osSVG(f);
    const udot = (t.kind === 'console' && unread.has(t.num)) ? '<span class="utab"></span>' : '';
    d.innerHTML = udot + icon + esc(label(t.num)) + '<span class="pop" title="Pop out into its own window">⧉</span><span class="x">&times;</span>';
    d.onclick = () => { active = tabKey(t); if (t.kind === 'console') unread.delete(t.num); renderTabs(); renderFriendList(); render(); };
    d.querySelector('.pop').onclick = e => { e.stopPropagation(); popOut(t); };
    d.querySelector('.x').onclick = e => { e.stopPropagation(); closeTab(t.num, t.kind); };
    d.draggable = true;
    d.ondragstart = e => { dragTab = t; e.dataTransfer.effectAllowed = 'move'; d.classList.add('dragging'); };
    d.ondragend = () => { dragTab = null; renderTabs(); };
    d.ondragover = e => {
      if (!dragTab || dragTab === t) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
      const r = d.getBoundingClientRect();
      const after = e.clientX > r.left + r.width / 2;
      d.classList.toggle('drop-before', !after);
      d.classList.toggle('drop-after', after);
    };
    d.ondragleave = () => d.classList.remove('drop-before', 'drop-after');
    d.ondrop = e => {
      e.preventDefault();
      d.classList.remove('drop-before', 'drop-after');
      if (!dragTab || dragTab === t) return;
      const r = d.getBoundingClientRect();
      moveTab(dragTab, t, e.clientX > r.left + r.width / 2);
    };
    el.appendChild(d);
  }
}

function activeTab() { return tabs.find(t => tabKey(t) === active) || null; }
function render() {
  const t = activeTab();
  const con = document.getElementById('console'), br = document.getElementById('browser'), cp = document.getElementById('compose');
  if (!t) { con.style.display = 'block'; br.style.display = 'none'; cp.style.display = 'none'; con.innerHTML = '<div id="empty">Click a friend to open a console</div>'; return; }
  if (t.kind === 'files') { con.style.display = 'none'; cp.style.display = 'none'; br.style.display = 'flex'; renderBrowser(t.num); return; }
  con.style.display = 'block'; br.style.display = 'none'; cp.style.display = 'flex';
  renderConsole(t.num);
}
function renderConsole(num) {
  const box = document.getElementById('console');
  const mine = msgs.filter(m => m.friend === num);
  const atBottom = box.scrollHeight - box.scrollTop - box.clientHeight < 40;
  box.innerHTML = '';
  if (!mine.length) { const d = document.createElement('div'); d.id = 'empty'; d.textContent = 'No messages yet.'; box.appendChild(d); return; }
  for (const m of mine) {
    const d = document.createElement('div');
    d.className = 'line ' + m.dir;
    d.textContent = m.text;
    d.title = new Date(m.time).toLocaleString();
    box.appendChild(d);
  }
  if (atBottom) box.scrollTop = box.scrollHeight;
}

/* ---- file browser ---- */
function fbState(num) {
  let s = fb.get(num);
  if (!s) { s = { path: '', os: '', entries: [], history: [], hi: -1, loading: false, error: '', jobs: {} }; fb.set(num, s); }
  return s;
}
async function fbLoad(num, path, push) {
  const s = fbState(num);
  s.loading = true; s.error = '';
  if (active === 'files' + num) renderBrowser(num);
  try {
    const r = await api('/api/files/' + num + '?path=' + encodeURIComponent(path));
    s.path = r.path; s.os = r.os; s.entries = r.entries || [];
    if (push) { s.history = s.history.slice(0, s.hi + 1); s.history.push(r.path); s.hi = s.history.length - 1; }
  } catch (err) { s.error = err.message; }
  s.loading = false;
  if (active === 'files' + num) renderBrowser(num);
}
function fbJoin(path, name, os) {
  const sep = os === 'windows' ? '\\' : '/';
  return path.endsWith(sep) ? path + name : path + sep + name;
}
function fbParent(p, os) {
  const sep = os === 'windows' ? '\\' : '/';
  let s = p.length > 1 ? p.replace(new RegExp('\\' + sep + '+$'), '') : p;
  const i = s.lastIndexOf(sep);
  if (i < 0) return p;
  if (os === 'windows') return i <= 2 ? s.slice(0, 3) : s.slice(0, i);
  return i === 0 ? '/' : s.slice(0, i);
}
function fmtBytes(n) {
  if (n < 1024) return n + ' B';
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
  if (n < 1073741824) return (n / 1048576).toFixed(1) + ' MB';
  return (n / 1073741824).toFixed(2) + ' GB';
}
function dlControls(job) {
  if (job.status === 'done') return '<a class="save" href="/api/download-file/' + job.id + '" download onclick="event.stopPropagation()">Save</a>';
  if (job.status === 'error') return '<span class="dlerr" title="' + esc(job.error || '') + '">failed</span>';
  if (job.status === 'cancelled') return '<span class="dlerr">cancelled</span>';
  let b = '';
  if (job.status === 'paused') b += '<button class="dlbtn" data-act="resume" data-id="' + job.id + '">Resume</button>';
  else b += '<button class="dlbtn" data-act="pause" data-id="' + job.id + '">Pause</button>';
  b += '<button class="dlbtn cancel" data-act="cancel" data-id="' + job.id + '">Cancel</button>';
  return b;
}
function progBar(job) {
  const pct = job.total > 0 ? Math.min(100, Math.floor(job.received * 100 / job.total)) : 0;
  const txt = job.status === 'paused' ? 'paused' : (job.total > 0 ? pct + '%' : fmtBytes(job.received));
  return '<span class="prog"><span class="bar" style="width:' + pct + '%"></span></span><span class="pct">' + txt + '</span>';
}
function jobWidget(job) {
  if (job.status === 'done' || job.status === 'error' || job.status === 'cancelled') return dlControls(job);
  return progBar(job) + dlControls(job);
}

/* ---- downloads store ---- */
let downloads = [];
function fmtWhen(t) {
  if (!t) return '';
  const d = new Date(t);
  if (isNaN(d.getTime())) return '';
  const diff = Date.now() - d.getTime();
  if (diff < 60000) return 'just now';
  if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago';
  if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago';
  return d.toLocaleString();
}
function updateDlBadge() {
  const n = downloads.filter(d => d.status === 'running' || d.status === 'paused').length;
  const b = document.getElementById('dlbadge');
  b.textContent = n ? n : '';
  b.style.display = n ? 'inline' : 'none';
}
function storeIsOpen() { return document.getElementById('store').classList.contains('open'); }
async function pollDownloads() {
  try { downloads = await api('/api/downloads') || []; } catch (e) { return; }
  updateDlBadge();
  if (storeIsOpen()) renderStore();
}
function renderStore() {
  const list = document.getElementById('storelist');
  if (!downloads.length) { list.innerHTML = '<div class="storeempty">No downloads yet.</div>'; return; }
  const scroll = list.scrollTop;
  list.innerHTML = '';
  for (const j of downloads) {
    const row = document.createElement('div');
    row.className = 'drow';
    const size = j.total > 0 ? fmtBytes(j.total) : (j.received > 0 ? fmtBytes(j.received) : '');
    const sub = '<span class="frm">' + esc(j.friendName || ('#' + j.friend)) + '</span> · ' +
      esc(j.path || '') + ' · ' + fmtWhen(j.time);
    let right = '';
    if (j.status === 'running' || j.status === 'paused') right += progBar(j);
    right += dlControls(j);
    row.innerHTML = '<div class="dmeta"><div class="dname">' + esc(j.name || '') + '</div>' +
      '<div class="dsub">' + sub + '</div></div>' +
      '<span class="dsize">' + size + '</span><span class="dright">' + right + '</span>';
    list.appendChild(row);
  }
  list.scrollTop = scroll;
}
async function dlControl(id, act) {
  try { await api('/api/download/' + id + '/' + act, { method: 'POST' }); }
  catch (e) { alert(e.message); }
  pollDownloads();
  const t = activeTab();
  if (t && t.kind === 'files') pollJobs(t.num);
}
function renderBrowser(num) {
  const s = fbState(num);
  document.getElementById('fbpath').textContent = s.path || 'loading…';
  document.getElementById('fbback').disabled = s.hi <= 0;
  document.getElementById('fbfwd').disabled = s.hi >= s.history.length - 1;
  const list = document.getElementById('fblist');
  const scroll = list.scrollTop;
  list.innerHTML = '';
  if (s.loading) { list.innerHTML = '<div class="fbmsg">Loading ' + esc(s.path || '') + ' …</div>'; return; }
  if (s.error) { list.innerHTML = '<div class="fbmsg">' + esc(s.error) + '</div>'; return; }
  if (!s.entries.length) { list.innerHTML = '<div class="fbmsg">(empty)</div>'; return; }
  const dirs = s.entries.filter(e => e.dir), fils = s.entries.filter(e => !e.dir);
  for (const e of dirs.concat(fils)) {
    const d = document.createElement('div');
    d.className = 'ent' + (e.dir ? ' dir' : '');
    const full = fbJoin(s.path, e.name, s.os);
    let html = (e.dir ? FOLDER_SVG : FILE_SVG) + '<span class="nm">' + esc(e.name) + '</span>';
    if (!e.dir && s.jobs[full]) html += jobWidget(s.jobs[full]);
    d.innerHTML = html;
    if (e.dir) {
      d.onclick = () => fbLoad(num, full, true);
    } else {
      d.oncontextmenu = ev => { ev.preventDefault(); dlPath = { num, path: full }; showFbMenu(ev.clientX, ev.clientY); };
      d.ondblclick = () => startDownload(num, full);
      d.title = 'right-click to download';
    }
    list.appendChild(d);
  }
  list.scrollTop = scroll;
}
async function startDownload(num, path) {
  const s = fbState(num);
  try {
    const job = await api('/api/download/' + num, { method: 'POST', body: JSON.stringify({ path }) });
    s.jobs[path] = job;
    if (active === 'files' + num) renderBrowser(num);
  } catch (err) { alert(err.message); }
}
async function pollJobs(num) {
  try {
    const list = await api('/api/download-jobs/' + num);
    const s = fbState(num);
    const m = {};
    for (const j of list) m[j.path] = j;
    s.jobs = m;
    if (active === 'files' + num) renderBrowser(num);
  } catch (e) {}
}
document.getElementById('fbback').onclick = () => { const t = activeTab(); if (!t) return; const s = fbState(t.num); if (s.hi > 0) { s.hi--; fbLoad(t.num, s.history[s.hi], false); } };
document.getElementById('fbfwd').onclick = () => { const t = activeTab(); if (!t) return; const s = fbState(t.num); if (s.hi < s.history.length - 1) { s.hi++; fbLoad(t.num, s.history[s.hi], false); } };
document.getElementById('fbup').onclick = () => { const t = activeTab(); if (!t) return; const s = fbState(t.num); fbLoad(t.num, fbParent(s.path, s.os), true); };

async function poll() {
  try {
    const nm = await api('/api/messages?since=' + cursor);
    if (nm && nm.length) {
      msgs.push(...nm);
      cursor = nm[nm.length - 1].id;
      // Flag a friend as unread when a message/output arrives and we're not viewing it.
      let changed = false;
      for (const m of nm) {
        if (m.dir !== 'in') continue;
        const viewing = active === 'console' + m.friend || popped['console' + m.friend];
        if (!viewing && !unread.has(m.friend)) { unread.add(m.friend); changed = true; }
      }
      if (changed) { renderFriendList(); renderTabs(); }
      if (popNotify) popNotify(nm);
      const t = activeTab();
      if (t && t.kind === 'console' && nm.some(m => m.friend === t.num)) render();
    }
  } catch (e) {}
}

/* ---- add friend dialog ---- */
const addmodal = document.getElementById('addmodal');
function openAdd() {
  addmodal.classList.add('open');
  document.getElementById('adderr').textContent = '';
  const i = document.getElementById('toxid'); i.value = ''; i.focus();
}
function closeAdd() { addmodal.classList.remove('open'); }
document.getElementById('addbtn').onclick = openAdd;
document.getElementById('addclose').onclick = closeAdd;
addmodal.onclick = e => { if (e.target === addmodal) closeAdd(); };
document.addEventListener('keydown', e => { if (e.key === 'Escape') closeAdd(); });
document.getElementById('addform').onsubmit = async e => {
  e.preventDefault();
  const i = document.getElementById('toxid');
  try {
    await api('/api/friends', { method: 'POST', body: JSON.stringify({ toxID: i.value.trim() }) });
    closeAdd();
    friendCat = 'people';   // an added (or claimed) friend lands in People; show it there
    document.querySelectorAll('.frtab').forEach(b => b.classList.toggle('active', b.dataset.cat === 'people'));
    loadFriends();
  } catch (err) { document.getElementById('adderr').textContent = err.message; }
};
document.getElementById('compose').onsubmit = async e => {
  e.preventDefault();
  const i = document.getElementById('text'); const text = i.value; const t = activeTab();
  if (!text || !t || t.kind !== 'console') return;
  try { await api('/api/send', { method: 'POST', body: JSON.stringify({ friend: t.num, text }) }); i.value = ''; await poll(); }
  catch (err) { alert(err.message); }
};

document.getElementById('handle').onmousedown = e => {
  e.preventDefault();
  const move = ev => { dockH = window.innerHeight - ev.clientY; updateDock(); };
  const up = () => {
    document.removeEventListener('mousemove', move); document.removeEventListener('mouseup', up);
    dockH = Math.max(120, Math.min(window.innerHeight - 160, dockH));
    try { localStorage.setItem('dockH', dockH); } catch (e) {}
  };
  document.addEventListener('mousemove', move); document.addEventListener('mouseup', up);
};
window.addEventListener('resize', updateDock);

/* ---- context menus ---- */
const menu = document.getElementById('menu'), fbmenu = document.getElementById('fbmenu');
function showMenu(cx, cy, num) {
  menuNum = num;
  document.getElementById('submenu').classList.remove('open');
  const f = friends.find(fr => fr.num === num);
  const person = !!(f && f.addedByUs);   // People get a single Open Chat; shells get the Interact submenu
  document.getElementById('mi-interact').style.display = person ? 'none' : '';
  document.getElementById('mi-chat').style.display = person ? '' : 'none';
  place(menu, cx, cy);
}
function showFbMenu(x, y) { place(fbmenu, x, y); }
function place(m, x, y) { m.style.left = x + 'px'; m.style.top = y + 'px'; m.style.display = 'block'; }
function hideMenus() { menu.style.display = 'none'; fbmenu.style.display = 'none'; document.getElementById('dlmenu').style.display = 'none'; document.getElementById('submenu').classList.remove('open'); }
// Keep the Interact submenu open with a short hide delay, so moving onto it
// (even diagonally) never closes it pre-emptively.
(() => {
  const mi = document.getElementById('mi-interact'), sub = document.getElementById('submenu');
  let t = null;
  const show = () => { clearTimeout(t); sub.classList.add('open'); };
  const hideSoon = () => { clearTimeout(t); t = setTimeout(() => sub.classList.remove('open'), 350); };
  mi.addEventListener('mouseenter', show);
  mi.addEventListener('mouseleave', hideSoon);
  sub.addEventListener('mouseenter', () => clearTimeout(t));
  sub.addEventListener('mouseleave', hideSoon);
})();
document.addEventListener('click', hideMenus);
document.addEventListener('scroll', hideMenus, true);
document.addEventListener('keydown', e => { if (e.key === 'Escape') hideMenus(); });
document.getElementById('mi-console').onclick = e => { e.stopPropagation(); const n = menuNum; hideMenus(); if (n !== null) openTab(n, 'console'); };
document.getElementById('mi-files').onclick = e => { e.stopPropagation(); const n = menuNum; hideMenus(); if (n !== null) openTab(n, 'files'); };
document.getElementById('mi-chat').onclick = e => { e.stopPropagation(); const n = menuNum; hideMenus(); if (n !== null) openTab(n, 'console'); };
document.getElementById('mi-dl').onclick = e => { e.stopPropagation(); const d = dlPath; hideMenus(); if (d) startDownload(d.num, d.path); };
document.getElementById('rm').onclick = async e => {
  e.stopPropagation();
  const num = menuNum; hideMenus();
  if (num === null) return;
  if (!confirm('Remove ' + label(num) + '?')) return;
  try { await api('/api/friends/' + num, { method: 'DELETE' }); closeTab(num, 'console'); closeTab(num, 'files'); loadFriends(); }
  catch (err) { alert(err.message); }
};

/* ---- activity log ---- */
const logsmodal = document.getElementById('logsmodal');
const textmodal = document.getElementById('textmodal');
let logsTab = 'downloads', logsPage = 0;
const LOGS_SIZE = 100;
function fmtLogTime(v) { if (!v) return ''; const d = new Date(v); return isNaN(d.getTime()) ? v : d.toLocaleString(); }
function keyShort(k) { return k ? k.slice(0, 16) + '…' : ''; }
// The console list shows the command that was run; command output is not shown
// here - click the row to view it.
function consoleCell(v, r) {
  if (r.direction !== 'out') return '↳ output';
  v = v || '';
  return v + (r.len > v.length ? ' …' : '');
}
const logCols = {
  downloads:   [['id','ID'],['friend_num','Friend'],['name','Name'],['remote_path','From'],['size','Size',fmtBytes],['status','Status'],['started_at','Started',fmtLogTime],['finished_at','Finished',fmtLogTime]],
  console:     [['at','Time',fmtLogTime],['num','Friend'],['text','Command',consoleCell,'wrap']],
  friends:     [['num','Friend'],['public_key','Public key',keyShort],['first_seen','First seen',fmtLogTime],['removed_at','Removed',fmtLogTime]],
  connections: [['at','Time',fmtLogTime],['num','Friend'],['public_key','Public key',keyShort],['online','State',v => (v == 1 || v === true) ? 'online' : 'offline']],
};
async function loadLogs(tab, page) {
  logsTab = tab; logsPage = Math.max(0, page || 0);
  for (const t of document.querySelectorAll('.ltab')) t.classList.toggle('active', t.dataset.t === tab);
  const body = document.getElementById('logsbody');
  let rows;
  try { rows = await api('/api/logs/' + tab + '?page=' + logsPage + '&size=' + LOGS_SIZE) || []; }
  catch (e) { body.innerHTML = '<div class="logempty">' + esc(e.message) + '</div>'; return; }
  const cols = logCols[tab], clickable = tab === 'console';
  let html = '<table class="logt"><thead><tr>' + cols.map(c => '<th>' + c[1] + '</th>').join('') + '</tr></thead><tbody>';
  if (!rows.length) html += '<tr><td colspan="' + cols.length + '" class="logempty">' + (logsPage ? 'No more entries.' : 'No entries yet.') + '</td></tr>';
  for (const r of rows) {
    const attr = clickable ? ' class="clickrow" data-cid="' + r.id + '" title="click to view full text"' : '';
    html += '<tr' + attr + '>' + cols.map(c => {
      let v = r[c[0]], cls = c[3] ? ' class="' + c[3] + '"' : '';
      if (typeof c[2] === 'function') v = c[2](v, r);
      else v = (v == null) ? '' : String(v);
      return '<td' + cls + '>' + esc(String(v)) + '</td>';
    }).join('') + '</tr>';
  }
  body.innerHTML = html + '</tbody></table>';
  document.getElementById('logsprev').disabled = logsPage === 0;
  document.getElementById('logsnext').disabled = rows.length < LOGS_SIZE;
  document.getElementById('logspage').textContent = 'Page ' + (logsPage + 1);
}
async function openConsoleText(id) {
  try {
    const row = await api('/api/logs/console/' + id);
    document.getElementById('texthd').textContent = (row.direction === 'out' ? '→ sent to' : '← from') + ' friend ' + row.num + '  ·  ' + fmtLogTime(row.at);
    document.getElementById('textbody').textContent = row.text || '';
    textmodal.classList.add('open');
  } catch (e) { alert(e.message); }
}
document.getElementById('logsbtn').onclick = () => { logsmodal.classList.add('open'); loadLogs(logsTab, logsPage); };
document.getElementById('logsclose').onclick = () => logsmodal.classList.remove('open');
logsmodal.onclick = e => { if (e.target === logsmodal) logsmodal.classList.remove('open'); };
document.getElementById('logsbody').onclick = e => { const tr = e.target.closest('tr.clickrow'); if (tr) openConsoleText(tr.dataset.cid); };
document.getElementById('logsprev').onclick = () => loadLogs(logsTab, logsPage - 1);
document.getElementById('logsnext').onclick = () => loadLogs(logsTab, logsPage + 1);
for (const t of document.querySelectorAll('.ltab')) t.onclick = () => loadLogs(t.dataset.t, 0);
document.getElementById('textclose').onclick = () => textmodal.classList.remove('open');
textmodal.onclick = e => { if (e.target === textmodal) textmodal.classList.remove('open'); };
document.addEventListener('keydown', e => {
  if (e.key !== 'Escape') return;
  if (textmodal.classList.contains('open')) textmodal.classList.remove('open');
  else logsmodal.classList.remove('open');
});

/* ---- downloads store wiring ---- */
const storeEl = document.getElementById('store');
document.getElementById('dlbtn').onclick = () => { storeEl.classList.add('open'); pollDownloads(); };
document.getElementById('storeclose').onclick = () => storeEl.classList.remove('open');
storeEl.onclick = e => { if (e.target === storeEl) storeEl.classList.remove('open'); };
document.addEventListener('keydown', e => { if (e.key === 'Escape') storeEl.classList.remove('open'); });
// Pause/resume/cancel buttons (in the store and the file browser) via delegation.
document.addEventListener('click', e => {
  const b = e.target.closest ? e.target.closest('.dlbtn') : null;
  if (!b) return;
  e.stopPropagation(); e.preventDefault();
  dlControl(b.dataset.id, b.dataset.act);
});

// Friends toolbar: category tabs, search box, sort key + direction.
document.querySelectorAll('.frtab').forEach(btn => btn.onclick = () => {
  friendCat = btn.dataset.cat;
  document.querySelectorAll('.frtab').forEach(b => b.classList.toggle('active', b === btn));
  renderFriendList();
});
document.getElementById('frsearch').oninput = e => { friendQuery = e.target.value; renderFriendList(); };
document.getElementById('frsort').onchange = e => { friendSort = e.target.value; renderFriendList(); };
document.getElementById('frdir').onclick = () => {
  friendDir = -friendDir;
  document.getElementById('frdir').textContent = friendDir < 0 ? '▼' : '▲';
  renderFriendList();
};

loadSelf();
loadFriends();
poll();
pollDownloads();
// Pop-out window: ?pop=<kind><num> opens just that one tab, full-window, no chrome.
(function () {
  const m = /^(console|files)(\d+)$/.exec(new URLSearchParams(location.search).get('pop') || '');
  if (!m) return;
  const kind = m[1], num = +m[2];
  document.body.classList.add('popped');
  openTab(num, kind);
  // Sticky header showing the exact same name the dock tab uses (label()).
  const hdr = document.getElementById('pophdr');
  let notify = false;   // a message arrived while this pop-out window was unfocused
  const setFavicon = () => {
    // A data: favicon is a standalone SVG, so add the xmlns the inline icons omit.
    let doc = osSVG(friends.find(x => x.num === num)).replace('<svg ', '<svg xmlns="http://www.w3.org/2000/svg" ');
    if (notify) doc = doc.replace('</svg>', '<circle cx="19" cy="5" r="5" fill="#4d9fff" stroke="#15171c" stroke-width="1.5"/></svg>');
    let link = document.querySelector('link[rel="icon"]');
    if (!link) { link = document.createElement('link'); link.rel = 'icon'; document.head.appendChild(link); }
    link.type = 'image/svg+xml';
    link.href = 'data:image/svg+xml,' + encodeURIComponent(doc);
  };
  const setHeader = () => {
    const f = friends.find(x => x.num === num);
    const icon = kind === 'files' ? FOLDER_SVG
      : '<span class="dot' + (isOnline(num) ? ' on' : '') + '"></span>' + osSVG(f);
    hdr.innerHTML = icon + esc(label(num));
    document.title = label(num) + ' - ' + (kind === 'files' ? 'Files' : 'Console');
    setFavicon();
  };
  window.addEventListener('focus', () => { if (notify) { notify = false; setFavicon(); } });
  popNotify = nm => {   // flag the favicon when output arrives and the window is in the background
    if (kind !== 'console' || document.hasFocus() || notify) return;
    if (nm.some(x => x.dir === 'in' && x.friend === num)) { notify = true; setFavicon(); }
  };
  setHeader(); setInterval(setHeader, 2000);
})();
setInterval(loadFriends, 2000);
setInterval(poll, 1000);
setInterval(pollDownloads, 1500);
setInterval(() => { const t = activeTab(); if (t && t.kind === 'files') pollJobs(t.num); }, 1000);
// When a popped-out window is closed, bring its tab back to the main dock.
setInterval(() => {
  for (const key in popped) {
    if (popped[key].win.closed) {
      const { num, kind } = popped[key];
      delete popped[key];
      openTab(num, kind);
    }
  }
}, 1000);
</script>
</body>
</html>`
