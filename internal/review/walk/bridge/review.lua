-- Review bridge: pointers and diff views driven over the nvim socket.
-- Loaded once at startup with nvim -c 'lua dofile(...)'.

local BASE = 'c76380d1e'
local NS = vim.api.nvim_create_namespace('review-bridge-pointer')
local last_pointer = nil  -- { path, lnum, endl, bufnr }
local DEBUG = '/Users/dreuss/tmp/logs/review-nvim-debug.log'

local function dlog(msg)
  local f = io.open(DEBUG, 'a')
  if f then
    f:write(os.date('%H:%M:%S') .. ' ' .. msg .. '\n')
    f:close()
  end
end

local function clear_highlight(buf)
  pcall(vim.api.nvim_buf_clear_namespace, buf, NS, 0, -1)
end

local applied = {}

local function apply_highlight(buf, lnum, endl, why)
  return nil
end

local function apply_highlight_disabled(buf, lnum, endl, why)
  if not lnum or lnum < 1 then return end
  local sig = tostring(lnum) .. ':' .. tostring(endl or 0)
  if applied[buf] == sig then return end
  applied[buf] = sig
  dlog('apply buf=' .. buf .. ' ' .. lnum .. '-' .. (endl or lnum) .. ' why=' .. (why or '?'))
  pcall(vim.api.nvim_buf_clear_namespace, buf, NS, 0, -1)
  local last = endl or lnum
  local lines = vim.api.nvim_buf_get_lines(buf, lnum - 1, last, false)
  for i, text in ipairs(lines) do
    if #text > 0 then
      pcall(vim.api.nvim_buf_add_highlight, buf, NS, 'ReviewPointer', lnum - 1 + i - 1, 0, #text)
    end
  end
end

local function open_path(path, lnum, end_lnum)
  dlog('open_path ' .. path .. ' ' .. tostring(lnum) .. '-' .. tostring(end_lnum))
  vim.cmd('edit ' .. vim.fn.fnameescape(path))
  local buf = vim.api.nvim_get_current_buf()
  local win = vim.api.nvim_get_current_win()
  if lnum and lnum > 0 then
    vim.api.nvim_win_set_cursor(win, { lnum, 0 })
    vim.cmd('normal! zz')
  end
  apply_highlight(buf, lnum, end_lnum, 'open_path')
  last_pointer = { path = path, lnum = lnum, endl = end_lnum, bufnr = buf }
  vim.wo[win].signcolumn = 'auto'
end

-- Keep each visited pointer lit in its own buffer. A new pointer on the
-- same buffer replaces it. BufEnter re-lights the last pointer so it
-- survives leaving and returning.
local function stick(buf)
  if last_pointer and last_pointer.bufnr == buf then
    apply_highlight(buf, last_pointer.lnum, last_pointer.endl, 'stick')
  end
end

local aug = vim.api.nvim_create_augroup('review_bridge', { clear = true })
vim.api.nvim_create_autocmd({ 'BufEnter', 'BufReadPost' }, {
  group = aug, callback = function(args) stick(args.buf) end,
})
vim.api.nvim_set_hl(0, 'ReviewPointer', { link = 'Visual' })
vim.api.nvim_create_user_command('ReviewClear', function()
  for _, b in ipairs(vim.fn.getbufinfo()) do
    clear_highlight(b.bufnr)
  end
  last_pointer = nil
end, { nargs = 0 })

local function scratch_with_contents(name, contents)
  local existing = vim.fn.bufnr(name)
  local buf = existing > 0 and existing or vim.api.nvim_create_buf(false, true)
  if existing <= 0 then
    vim.api.nvim_buf_set_name(buf, name)
  end
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, vim.split(contents, '\n', { plain = true }))
  vim.bo[buf].buftype = 'nowrite'
  vim.bo[buf].bufhidden = 'hide'
  vim.bo[buf].modified = false
  return buf
end

local diffwins = nil

local function diff_base(path, lnum, endl)
  pcall(vim.cmd, 'DiffviewClose')
  local out = vim.fn.system('git show ' .. BASE .. ':' .. path)
  if vim.v.shell_error ~= 0 then
    out = ''
  end
  local cur_win = vim.api.nvim_get_current_win()
  for _, w in ipairs(vim.api.nvim_list_wins()) do
    if w ~= cur_win and vim.api.nvim_win_get_config(w).relative == '' then
      pcall(vim.api.nvim_win_close, w, true)
    end
  end
  vim.api.nvim_win_set_cursor(cur_win, { math.max(lnum or 1, 1), 0 })
  vim.cmd('leftabove vertical split')
  local base_win = vim.api.nvim_get_current_win()
  local base_buf = vim.fn.bufnr('base:' .. path, 1)
  vim.api.nvim_buf_set_lines(base_buf, 0, -1, false,
                             vim.split(out, '\n', { plain = true }))
  vim.bo[base_buf].buftype = 'nowrite'
  vim.bo[base_buf].bufhidden = 'hide'
  vim.bo[base_buf].modified = false
  vim.bo[base_buf].filetype = vim.filetype.match({ filename = path }) or 'text'
  vim.api.nvim_win_set_buf(base_win, base_buf)
  vim.cmd('diffthis')
  vim.api.nvim_set_current_win(cur_win)
  vim.cmd('diffthis')
  vim.cmd('normal! zz')
end

vim.api.nvim_create_user_command('Review', function(opts)
  local args = vim.split(opts.args, '%s+')
  local path = args[1]
  if not path or path == '' then
    vim.notify('Review needs a path', vim.log.levels.ERR)
    return
  end
  local lnum = tonumber(args[2]) or 0
  local endlnum = tonumber(args[3]) or lnum
  open_path(path, lnum, endlnum)
end, { nargs = '+' })

local function wait_hunks(base)
  local gs = require('gitsigns')
  vim.wait(4000, function()
    local ok, h = pcall(gs.get_hunks)
    return ok and h ~= nil
  end, 150)
  if base and base ~= '' then
    pcall(gs.change_base, base, false)
  end
  local hunks = {}
  vim.wait(6000, function()
    local ok, h = pcall(gs.get_hunks)
    hunks = (ok and h) or {}
    return #hunks > 0
  end, 300)
  return hunks
end

local function pick_hunk(hunks, lnum, endl)
  for _, h in ipairs(hunks) do
    local s = h.added.start
    local e = s + math.max(h.added.count - 1, 0)
    if s <= endl and e >= lnum then
      return h
    end
  end
  local dist = math.huge
  local best = nil
  for _, h in ipairs(hunks) do
    local d = math.abs(h.added.start - lnum)
    if d < dist then
      dist = d
      best = h
    end
  end
  return best
end

local function show_preview(lnum, endl, base)
  local gs = require('gitsigns')
  local hunks = wait_hunks(base)
  local best = pick_hunk(hunks, lnum, endl)
  if not best then
    vim.notify('no changes vs ' .. (base or 'HEAD'), vim.log.levels.WARN)
    return
  end
  if best.added.count <= 40 then
    vim.api.nvim_win_set_cursor(0, { best.added.start, 0 })
  end
  vim.cmd('normal! zz')
  for _ = 1, 10 do
    for _, w in ipairs(vim.api.nvim_list_wins()) do
      if vim.api.nvim_win_get_config(w).relative ~= '' then
        pcall(vim.api.nvim_win_close, w, true)
      end
    end
    if pcall(gs.preview_hunk) then
      break
    end
    vim.wait(300)
  end
end

vim.api.nvim_create_user_command('ReviewPreview', function(opts)
  local args = vim.split(vim.trim(opts.args), '%s+')
  local lnum = tonumber(args[1]) or vim.fn.line('.')
  local endl = tonumber(args[2]) or lnum
  show_preview(lnum, endl, args[3])
end, { nargs = '*' })

function _G.ReviewPerCommit()
  pcall(vim.cmd, 'DiffviewClose')
  vim.cmd('DiffviewFileHistory ' .. BASE .. '..HEAD')
end

local function stamp_commit_note(sha, note)
  vim.defer_fn(function()
    note = (note or ''):gsub('\n', ' ')
    local w = math.max(vim.fn.strdisplaywidth(note), 16)
    local top = '╭ L' .. string.rep('─', 2) .. '╮'
    local lines = {
      { { '╭' .. string.rep('─', w + 2) .. '╮', 'ReviewNote' } },
      { { '│ ' .. note .. string.rep(' ', math.max(w - vim.fn.strdisplaywidth(note), 0)) .. ' │', 'ReviewNote' } },
      { { '╰' .. string.rep('─', w + 2) .. '╯', 'ReviewNote' } },
    }
    for _, b in ipairs(vim.api.nvim_list_bufs()) do
      local name = vim.fn.bufname(b)
      if name:find('diffview://', 1, true) and name:find(sha:sub(1, 10), 1, true) and vim.api.nvim_buf_line_count(b) > 0 then
        pcall(vim.api.nvim_buf_clear_namespace, b, NS, 0, -1)
        pcall(vim.api.nvim_buf_set_extmark, b, NS, 0, 0, {
          virt_lines = lines, virt_lines_above = true,
        })
      end
    end
  end, 1200)
end

function _G.ReviewCommit(sha, note)
  pcall(vim.cmd, 'DiffviewClose')
  vim.cmd('DiffviewOpen ' .. sha .. '^..' .. sha)
  if note then
    stamp_commit_note(sha, note)
  end
end

function _G.ReviewDiffCurrent()
  local last = _G.__review_last
  if not last or not last.path then
    vim.notify('no file shown yet', vim.log.levels.WARN)
    return
  end
  vim.cmd('DiffviewOpen ' .. BASE)
end

local function set_note(buf, from, endl, note)
  pcall(vim.api.nvim_buf_clear_namespace, buf, NS, 0, -1)
  note = (note or ''):sub(1, 110)
  local last = vim.api.nvim_buf_line_count(buf)
  local from_l = math.min(math.max(tonumber(from) or 1, 1), last)
  local endl_l = math.min(math.max(tonumber(endl) or from_l, from_l), last)
  local w = math.max(vim.fn.strdisplaywidth(note), 16)
  local label = (' L%d-%d '):format(from_l, endl_l)
  local top = '╭' .. label .. string.rep('─', math.max(w - #label + 2, 2)) .. '╮'
  local mid = '│ ' .. note .. string.rep(' ', math.max(w - vim.fn.strdisplaywidth(note), 0)) .. ' │'
  local bot = '╰' .. string.rep('─', w + 2) .. '╯'
  pcall(vim.api.nvim_buf_set_extmark, buf, NS, from_l - 1, 0, {
    virt_lines = { { { top, 'ReviewNote' } }, { { mid, 'ReviewNote' } } },
    virt_lines_above = true,
  })
  pcall(vim.api.nvim_buf_set_extmark, buf, NS, endl_l - 1, 0, {
    virt_lines = { { { bot, 'ReviewNote' } } },
    virt_lines_above = false,
  })
end

function _G.ReviewGo(path, lnum, endl, base, note)
  pcall(vim.cmd, 'DiffviewClose')
  open_path(path, tonumber(lnum), tonumber(endl))
  _G.__review_last = { path = path, lnum = tonumber(lnum), endl = tonumber(endl) }
  for _, w in ipairs(vim.api.nvim_list_wins()) do
    if vim.api.nvim_win_get_config(w).relative ~= '' then
      pcall(vim.api.nvim_win_close, w, true)
    end
  end
  local gs = require('gitsigns')
  if base and base ~= '' then
    pcall(gs.change_base, base, false)
  end
  local ns = vim.api.nvim_get_namespaces()['gitsigns_signs_']
  vim.wait(6000, function()
    local n = ns and #vim.api.nvim_buf_get_extmarks(0, ns, 0, -1, {}) or 0
    if n == 0 and base and base ~= '' then
      pcall(gs.change_base, base, false)
    end
    return n > 0
  end, 250)
  -- Attention dial: 'none' = cursor only, 'cursorline' = subtle underline,
  -- 'visual' = native Visual selection over the range.
  vim.cmd([[execute "normal! \<Esc>"]])
  local from = math.max(tonumber(lnum) or 1, 1)
  local to = math.max(tonumber(endl) or from, from)
  local attention = _G.__review_attention or 'none'
  if attention == 'visual' then
    vim.cmd('normal! ' .. from .. 'GV' .. to .. 'G')
  else
    vim.api.nvim_win_set_cursor(0, { from, 0 })
    vim.wo.cursorline = (attention ~= 'none')
  end
  vim.cmd('normal! zz')
  set_note(vim.api.nvim_get_current_buf(), tonumber(lnum), tonumber(endl), note)
end

function _G.ReviewAttention(level)
  _G.__review_attention = level
end

function _G.ReviewShowDiff()
  local last = _G.__review_last
  if not last then
    vim.notify('no file shown yet', vim.log.levels.WARN)
    return
  end
  diff_base(last.path, last.lnum, last.endl)
end

vim.api.nvim_create_user_command('ReviewDiff', function(opts)
  local files = {}
  for f in vim.gsplit(vim.trim(opts.args), '%s+') do
    if f ~= '' then files[#files + 1] = f end
  end
  if #files == 0 then
    vim.notify('ReviewDiff needs a path', vim.log.levels.ERR)
    return
  end
  if pcall(require, 'diffview') then
    local cmd = 'DiffviewOpen ' .. BASE
    for _, f in ipairs(files) do
      cmd = cmd .. ' -- ' .. f
    end
    vim.cmd(cmd)
    return
  end
  diff_base(files[1], 0, 0)
end, { nargs = '+' })

vim.notify('Review bridge loaded: Review path [from] [to], ReviewDiff path', vim.log.levels.INFO)

-- Selection notes: visual-select lines, then <leader>n, type a comment.
local NOTES = '/Users/dreuss/tmp/pr595/review-notes.jsonl'

vim.api.nvim_create_user_command('ReviewNote', function(opts)
  local buf = vim.api.nvim_get_current_buf()
  local path = vim.fn.fnamemodify(vim.api.nvim_buf_get_name(buf), ':.')
  local from = vim.fn.line("'<")
  local to = vim.fn.line("'>")
  local entry = {
    file = path,
    from = tonumber(from),
    to = tonumber(to),
    comment = opts.args,
    base = BASE,
    time = os.date('%H:%M:%S'),
  }
  local f = io.open(NOTES, 'a')
  if f then
    f:write(json.encode(entry) .. '\n')
    f:close()
  end
  vim.notify('noted: ' .. path .. ' L' .. from .. '-' .. to, vim.log.levels.INFO)
end, { nargs = '+' })

vim.keymap.set('v', '<leader>n', ':<C-U>ReviewNote ', { silent = true })

vim.cmd('highlight Visual gui=none ctermbg=236 guibg=#30363d')
vim.cmd('highlight ReviewNote guifg=#e0af68 gui=none')

-- ReviewCapabilities: the frozen bridge contract. The controller checks it
-- before driving; a mismatch refuses cleanly.
function _G.ReviewCapabilities()
  return vim.json.encode({
    bridge_version = 1,
    nvim_api_version = vim.version().major .. '.' .. vim.version().minor .. '.' .. vim.version().patch,
    gitsigns = pcall(require, 'gitsigns'),
    diffview = pcall(require, 'diffview'),
  })
end
