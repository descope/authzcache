#!/usr/bin/env bash

CURRENT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)

# Run default linter for all projects, can be extended in each project, user --build-folder to specify build directoy
run_default_linter() {
	while [[ "$#" -gt 0 ]]; do
		case $1 in
		-bf | --build-folder)
			local BUILD_FOLDER="$2"
			;;
		-gc | --golangci-config)
			local GOLANGCI_CONFIG="$2"
			;;
		-dncm | --do-not-check-main)
			local DO_NOT_CHECK_MAIN="$2"
			;;
		esac
		shift
	done

	# if DO_NOT_CHECK_MAIN is set we run only secrets check (CI mode)
	if [ -z "$DO_NOT_CHECK_MAIN" ]; then
		lint_check_not_main
		lint_go_mod
		lint_go_build $BUILD_FOLDER
		lint_run_golangci $GOLANGCI_CONFIG
	fi
	lint_find_secrets
	lint_done
}

# Prevent pushing to main
lint_check_not_main() {
	echo "- Check branch protection"
	if [ -z "$1" ]; then
		protected_branch='main'
		current_branch=$(git symbolic-ref HEAD | sed -e 's,.*/\(.*\),\1,')
		if [ $protected_branch = $current_branch ]; then
			echo "pushing to main is not allowed"
			exit 1
		fi
	fi
}

# Run go mod commands
lint_go_mod() {
	echo "- Running go mod tidy"
	go mod tidy
	if [ -d "vendor" ]; then
		go_version=$(go version | cut -d " " -f3 | tr -d 'go')
		major_version=$(echo $go_version | cut -d "." -f1)
		minor_version=$(echo $go_version | cut -d "." -f2)

		if [ "$major_version" -eq 1 ] && [ "$minor_version" -ge 22 ] && [ -f ../go.work ]; then
			echo "- Running go work vendor"
			go work vendor
		else
			echo "- Running go mod vendor"
			go mod vendor
		fi
	fi
}

# Run go build (default is cmd dir)
lint_go_build() {
	local folder="${1:-"cmd"}" # get first argument and set "cmd" to be default
	echo "- Running go build for: ${folder}"
	go build ${folder}
	buildcount="$(echo $?)"
	if [ $buildcount -gt 0 ]; then
		echo "Project does not compile, run go build to check what are the errors"
		exit 1
	fi
}

# Run golangci-lint
lint_run_golangci() {
	echo "- Running golangci-lint"
	# renovate: datasource=github-releases depName=golangci/golangci-lint
	GOLANG_CI_SUPPORTED_VERSION="1.64.6"
	INSTALLED_GOLANG_CLI_VERSION="$(golangci-lint --version)"
	if [[ $INSTALLED_GOLANG_CLI_VERSION != *"$GOLANG_CI_SUPPORTED_VERSION"* ]]; then
		echo "Installing golangci-lint for the first time..."
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(go env GOPATH)/bin v${GOLANG_CI_SUPPORTED_VERSION}
		echo "Done downloading golangci-lint"
	fi

	local golang_cli_config="${1:-"${CURRENT_DIR}/.golangci.yml"}" # get first argument and set "cmd" to be default
	lintresult=$(golangci-lint --config ${golang_cli_config} --out-format colored-line-number run)
	lintcode=$?
	if [ $lintcode -ne 0 ]; then
		echo "golangci-lint failed to run with exit code $lintcode"
		echo "$lintresult"
		exit 1
	fi

	if [[ -n $lintresult ]]; then
		echo "Some files aren't passing lint, please run 'golangci-lint run' to see the errors it flags and correct your source code before committing"
		echo "$lintresult"
		exit 1
	fi
	echo "- Lint passed sucessfully!"
}

# Run detect-secrets
lint_find_secrets() {
	echo "- Running secrets check"
	SECRETS_SUPPORTED_VERSION="8.18.3"
	INSTALLED_SECRETS_VERSION="$(gitleaks version)"
	if [[ $INSTALLED_SECRETS_VERSION != *"$SECRETS_SUPPORTED_VERSION"* ]]; then
		echo "Installing gitleaks $(uname -s)_$(arch) for the first time..."
		iterations=0
		until [ "$iterations" -ge 10 ]; do
			if [ -n "$TOKEN" ]; then
				# Include the Authorization header with the Bearer token
				echo "Using TOKEN for headers"
				headers="Authorization: Bearer $TOKEN"
			fi

			# Make the cURL request
			FILE=$(curl --header "$headers" -s https://api.github.com/repos/gitleaks/gitleaks/releases/tags/v${SECRETS_SUPPORTED_VERSION} | jq -r "first(.assets[].name | select(test(\"$(uname -s)_$(arch)\"; \"i\") or test(\"$(uname -s)_x64\"; \"i\")))")
			if [ -z "$FILE" ]; then
				echo "Using Redirect URL"
				URL_REDIRECT=$(curl --header "$headers" -s https://api.github.com/repos/gitleaks/gitleaks/releases/tags/v${SECRETS_SUPPORTED_VERSION} | jq -r ".url")
				echo ${URL_REDIRECT}
				FILE=$(curl --header "$headers" -s ${URL_REDIRECT} | jq -r "first(.assets[].name | select(test(\"$(uname -s)_$(arch)\"; \"i\") or test(\"$(uname -s)_x64\"; \"i\")))")
			else
				echo "Using original URL"
			fi
			TMPDIR=$(mktemp -d)
			curl -o ${TMPDIR}/${FILE} -JL https://github.com/zricethezav/gitleaks/releases/download/v${SECRETS_SUPPORTED_VERSION}/${FILE}
			tar zxv -C "$(go env GOPATH)"/bin -f ${TMPDIR}/${FILE} gitleaks
			rm ${TMPDIR}/${FILE}

			gitleaks version && echo "Done installing gitleaks" && break

			iterations=$((iterations + 1))
			echo "Did not manage to install gitleaks, retrying. Attempt: $iterations"
			sleep 3
		done
	fi
	echo "  - Finding leaks in git log"
	gitleaks detect -v --redact -c ${CURRENT_DIR}/gitleaks.toml
	echo "gitleaks detect -v --redact -c ${CURRENT_DIR}/gitleaks.toml"
	if [ $? -ne 0 ]; then
		exit 1
	fi
	echo "  - Finding leaks in local repo"
	gitleaks detect --no-git -v --redact -c ${CURRENT_DIR}/gitleaks.toml
	if [ $? -ne 0 ]; then
		exit 1
	fi
	echo "- Secrets check passed successfully!"
}

# Indicates done
lint_done() {
	echo "Done!"
	exit 0
}

# Run Python Poetry
lint_run_python_poetry() {
	echo "- Running python linters"
	# Check if Poetry installed on the system
	if ! [ -x "$(command -v poetry)" ]; then
		echo "Installing Poetry for the first time..."
		curl -sSL https://install.python-poetry.org | python3 -

		ZSHRC_FILE=~/.zshrc
		if [ -f "$ZSHRC_FILE" ]; then
			# Add poetry to $PATH permanently
			echo '# Add Python Poetry path' >>$ZSHRC_FILE
			echo 'export PATH=$PATH:$HOME/.local/bin' >>$ZSHRC_FILE
		fi

		BASHRC_FILE=~/.bashrc
		if [ -f "$BASHRC_FILE" ]; then
			# Add poetry to $PATH permanently
			echo '# Add Python Poetry path' >>$BASHRC_FILE
			echo 'export PATH=$PATH:$HOME/.local/bin' >>$BASHRC_FILE
		fi

		# Reload $PATH env var
		export PATH=$PATH:$HOME/.local/bin

		echo "Done installing Poetry"
	fi

	#Isort
	echo "Running Python Isort linter"
	poetry run isort --profile black python/ || exit 1
	#Flake8
	echo "Running Python Flake8 linter"
	poetry run flake8 python/ || exit 1
	#Black
	echo "Running Python Black linter"
	poetry run black python/ || exit 1

	echo "- Python linters passed sucessfully!"
}
