# Manage Models Command

The `ollama model` command allows you to manage local models in Ollama, specifically supporting importing, exporting, and updating them.

## Usage

```bash
ollama model [flags]
```

## Available Commands

### Import a Model

You can import a model from a GGUF file path and optionally give it a description.

```bash
ollama model --import /path/to/model.gguf --name my-model:latest --description "An amazing new model."
```
* **Flags**:
  * `--import`: Path to the GGUF file.
  * `--name` or `-n`: The name for the imported model.
  * `--description`: Adds a description to the model.

### Export a Model

You can export an existing model to a specified destination.

```bash
ollama model --export my-model:latest --dest /path/to/export/dir/
```
* **Flags**:
  * `--export`: The name of the model you wish to export.
  * `--dest`: Destination file or directory for the GGUF file.

### Update a Model

You can update properties of an existing model, such as its description, without needing to re-import it.

```bash
ollama model --update my-model:latest --update-description "This is my updated description."
```
* **Flags**:
  * `--update`: The name of the existing model you wish to update.
  * `--update-description`: The new description for the model.

## Help

For a full list of commands and aliases, use:
```bash
ollama model --help
```
