# Blocks

##

Lines in an HTML block are HTML:

<pre>
# not a heading
.not a command
: not a note
// not a comment
</pre>

<div>
.caption not a caption
</div>

A comment does not end a list:

- one
// A comment.
- two

.caption A command ends one.
- three

- An indented line is text in a list item:
  .caption not a caption

A [reference][ref] works across commands.

.caption Caption.

[ref]: https://go.dev/

---
<h1>Blocks</h1>
<section>
<p>Lines in an HTML block are HTML:</p>
<pre>
# not a heading
.not a command
: not a note
// not a comment
</pre>
<div>
.caption not a caption
</div>
<p>A comment does not end a list:</p>
<ul>
<li>one</li>
<li>two</li>
</ul>
<figcaption>A command ends one.</figcaption>
<ul>
<li>
<p>three</p>
</li>
<li>
<p>An indented line is text in a list item:
.caption not a caption</p>
</li>
</ul>
<p>A <a href="https://go.dev/" target="_blank" rel="noopener">reference</a> works across commands.</p>
<figcaption>Caption.</figcaption>
</section>
