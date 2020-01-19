import React from 'react';
import './App.css';

class Display extends React.Component {
  render() {
    if (!this.props.data) {
      return (<h1>&nbsp;</h1>);
    } else {
      // Capitalize first letter of each word.
      const seniority = this.props.data.seniority.charAt(0).toUpperCase()
        + this.props.data.seniority.substring(1);
      const field = this.props.data.field.charAt(0).toUpperCase()
        + this.props.data.field.substring(1);
      const role = this.props.data.role.charAt(0).toUpperCase()
        + this.props.data.role.substring(1);

      return (
        <h1>{seniority} {field} {role}</h1>
      );
    }
  }
}

class Button extends React.Component {
  render() {
    return (
      <button
        className="btn btn-primary w-50"
        onClick={() => this.props.handler()}
      >Generate Fake Title</button>
    );
  }
}

class SlowButton extends React.Component {
  render() {
    return (
      <button
        className="btn btn-primary w-50"
        onClick={() => this.props.handler()}
      >Generate Fake Title Slowly</button>
    );
  }
}

class MyApp extends React.Component {
  constructor(props) {
    super(props);

    this.state = {
      data: null,
    };

    this.handler = this.handler.bind(this)
    this.slowHandler = this.slowHandler.bind(this)
  }

  handler() {
    fetch("http://localhost:8080/api")
      .then(res => res.json())
      .then(
        (result) => {
          this.setState({
            data: result,
          });
        },
        (error) => {
          this.setState({
            error
          });
        }
      );
  }

  slowHandler() {
    fetch("http://localhost:8080/slow-api")
      .then(res => res.json())
      .then(
        (result) => {
          this.setState({
            data: result,
          });
        },
        (error) => {
          this.setState({
            error
          });
        }
      );
  }

  render() {
    const { error, isLoaded, data } = this.state;
    if (error) {
      return <div>Error: {error.message}</div>;
    } else {
      return (
        <div className="App container">
          <Display data={data} />
          <div className="button-container my-1">
            <Button handler={this.handler} />
          </div>
          <div className="button-container my-1">
            <SlowButton handler={this.slowHandler} />
          </div>
        </div >
      );
    }
  }
}

function App() {
  return (
    <MyApp />
  );
}

export default App;
